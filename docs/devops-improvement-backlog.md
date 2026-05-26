# DevOps / Infra-Engineer Improvement Backlog

Improvements to help operators audit clusters, debug policy gaps, and answer
"why isn't this working?" without leaving the graph. Ordered by estimated
operator value vs. implementation cost. None are committed — backlog only.

---

## 1. Path query (reachability + pair trace)

**Problem.** Today the graph shows allow edges between workloads, but two
common operator questions have no direct answer:

- "Why is there *no* edge from A to B?" UI shows nothing — operator can't tell
  if it's default-allow with no policies, default-deny via an unrelated
  lockdown policy, or one engine quietly blocking.
- "What's the effective intersection of multiple engines for this specific
  pair?" Today the graph renders one edge per engine; operator has to AND them
  mentally for ports/directions.

**Two-depth UX.**

- **Reachability** (single click): select node → highlight every workload it
  can reach (effective intersection across engines); dim the rest.
- **Pair trace** (two clicks): select src then dst → panel shows
  per-engine verdict, contributing policies, port intersection, and the
  blocking factor when traffic doesn't flow.

**Backend.** `GET /api/path?src=<id>&dst=<id>` runs intersection logic
specific to that pair; returns structured trace.

**Why now.** Biggest single audit win. Backend already has every
engine's `Allow []Rule` — just needs an endpoint and the trace builder.

---

## 2. Orphan policy detection

**Problem.** A `NetworkPolicy` whose `podSelector` matches zero workloads is
silent dead code. Common failure modes: typo in selector, deployment renamed,
labels changed on the workload. Operator only notices when expected
restriction isn't restricting.

**Backend.** Each engine's `Evaluate` already iterates policies × workloads
via selector matching. Track which policies produce zero matches and add to
`EvaluationResult.OrphanedPolicies []PolicyRef`.

**UI.** Sidebar or warning section listing orphaned policies with
namespace + selector. Optionally annotate the policy on the graph.

**Why valuable.** Catches the entire class of "I wrote a policy and nothing
happened" bugs. Cheap to compute, no UX redesign needed.

---

## 3. Default-allow ("unprotected") visual

**Problem.** A workload with no policy selecting it is wide open in both
directions. Coverage tint (red) hints at this today, but competes with
namespace coloring and isn't a first-class signal.

**UI.** Explicit "unprotected" icon or badge on the node, separate from the
coverage color. Filter chip "show only unprotected" for fast triage.

**Why valuable.** Quickest answer to "which workloads in this cluster have
zero network policy protection?"

---

## 4. Peer match details in edge tooltip

**Problem.** Edge panel shows policy name + direction + ports, but not *how*
the policy matched (which label selector, which CIDR, which namespace
selector). Operator has to open the YAML to correlate.

**UI.** Detail panel includes "matched via" block: e.g.
`podSelector: app=frontend, tier=web` or `ipBlock: 10.0.0.0/8` or
`namespaceSelector: kubernetes.io/metadata.name=monitoring`.

**Backend.** `policy.Rule.Contributors` already has `RuleIndex` — extend
`PolicyRef` (or the rule itself) with a structured `PeerMatch` snapshot,
or look it up from the parsed policy in the response handler.

---

## 5. Policy-centric view (inverse of node view)

**Problem.** Current UI is node-centric: click workload → see policies
affecting it. Operators sometimes start from a policy name: "what does
`allow-frontend-ingress` actually do?" and want to see selected workloads +
produced edges highlighted.

**UI.** Policy picker (search by name/namespace). Selecting a policy:

- Highlights every workload its `podSelector` matches
- Highlights every edge produced by that policy
- Dims the rest

**Backend.** No new data — existing edges carry `policyName + policySource`
already; just need a filter mode in the frontend store.

---

## 6. Cross-engine "intersection drop" indicator

**Problem.** When k8s allows traffic but Istio denies (or vice versa),
the edge disappears from the effective view. Operator may not realize that
one engine's permission is being overridden by another.

**UI.** Mark the node or the would-be edge with a "conflict" icon:
"engine X allows, engine Y blocks — net: blocked." Click for details.

**Backend.** Compute per-pair: for each engine, what is the verdict? Where
they disagree, emit a `ConflictRecord{src, dst, allowingEngine,
blockingEngine, reason}`.

**Why valuable.** This is invisible today and is one of the harder
multi-engine debug scenarios.

---

## 7. State diff / snapshot mode

**Problem.** "I shipped a policy change. What did it change in the cluster?"
No easy answer without diffing JSON manually.

**UI.** Snapshot button → captures current graph state. Pick a previous
snapshot → diff view: added edges (green), removed edges (red), workloads
that gained/lost status keys.

**Backend.** Snapshots as compressed JSON blobs server-side, or
client-side IndexedDB. Optional — could be entirely client-side initially.

**Why valuable.** Pre/post-deploy verification is currently manual.

---

## 8. Policy YAML preview in detail panel

**Problem.** After identifying a relevant policy, operator copies its
name and runs `kubectl get netpol <name> -n <ns> -o yaml` in a terminal to
see the source. Slow context-switch.

**UI.** Detail panel includes a "View YAML" expander with the source
NetworkPolicy serialized.

**Backend.** New endpoint `GET /api/policy?source=k8s&namespace=foo&name=bar`
returns the raw object. Could be lazy-loaded on expand.

---

## Quick wins (lower scope, smaller individual value)

- **Edge port count badge** when bundling multiple policies between same pair
  (e.g. "5 policies, 3 unique ports").
- **Keyboard shortcuts**: `/` for filter focus, `Esc` to clear selection,
  `f` to fit view, `?` for shortcut list.
- **URL state**: persist selected node/edge + filter selections in the URL so
  operators can share a specific view ("look at this conflict").
- **Multi-select for bulk-filter**: shift-click multiple nodes to see only
  edges touching the selected set.
- **Stale-data indicator**: if cluster state is older than N seconds, show a
  "refresh" hint — relevant for live debugging during incidents.

---

## Notes

- Each item assumes the existing multi-engine architecture (`PolicySource`
  interface, per-engine `EvaluationResult`). None requires re-architecting.
- Items 1–3 cover the bulk of day-to-day audit pain. Items 4–8 add
  precision but assume the basics are in place.
