# Issues Feature — Review & Roadmap

Source: backend-sre review of the initial `/api/issues` implementation (2026-07-12).
Status: backlog — not started. Order of operations at the bottom.

---

## 1. What works today

**Policy rollout review.** Apply a new deny/baseline, reload, check drawer. Culprits
are `PolicyRef`s — the object an operator actually edits, not rule lines
(`dedupContributors`, `internal/store/reachability.go:101`). Cross-engine attribution
(`internal/store/buildIssues.go:147`) blames the blocking engine, not the permitting
one — istio-allows/k8s-denies is the real production case and most tools get it wrong.

**DNS breakage detection.** "Added netpol, everything times out 5s then fails" is the
single most common NetworkPolicy page. Detecting it proactively is high value.

**Partial-access demotion is load-bearing.** Default-deny baseline + per-app allows is
how every mature cluster runs netpol. Without `classifyLayering`
(`internal/store/layering.go:35`) the drawer would be 90% false conflicts and nobody
would open it twice. Demote-to-info with evidence (which fine tier composes with the
coarse block) is the decision that makes the whole feature usable.

**Where it falls short:**

- 3am pages are symptom-driven ("app→db broken"); drawer is inventory-driven. The
  reachability panel serves the symptom journey — fine. But the drawer answers no
  "what changed since yesterday" question either. Inventory without delta gets read
  once at rollout, then never again.
- Coverage is silently scoped to whatever namespaces got browsed into the cache
  (population only via `/api/graph`, `internal/api/network-policy.go:26`). Load 2
  namespaces, open drawer → "cluster has 3 issues" actually means "3 issues among
  what someone happened to browse".

---

## 2. Detectors to add next (ranked value/cost)

### 2.1 Node lockout — do first

**What:** per workload, run `decideDirectionVerdict` over its own `NodeRules` buckets
in both directions; default-deny/locked with zero allows both ways = locked out.
O(nodes), no pair sweep, rides existing plumbing end to end.

**Why it matters:** "applied default-deny, forgot the allow rules" is the #1 policy
rollout mistake in practice. It takes down a workload completely and the operator's
first signal is usually the app team paging them. Catching it at review time — before
traffic dies — converts a 3am page into a 30-second fix during rollout. The type is
already declared (`internal/models/issues.go:10`) and the UI severity is mapped
(`ui/src/data/policies.ts:249`); today the filter chip exists but can never fire,
which actively misleads (see 4.5).

### 2.2 Mesh conflict

**What:** second pair sweep with mesh sources enabled. `IsNodesReachable` already runs
the mesh pass (`internal/store/reachability.go:214`); `PolicyIssues` deliberately
passes `noMesh` (`internal/store/buildIssues.go:92`). Flag pairs where engines allow
but mesh denies (STRICT dst, off-mesh src).

**Why it matters:** the cluster is ambient-mode mid-migration — exactly the state where
L3/L4 policy says "allowed" but mTLS strictness silently kills the connection. This
class of failure is invisible in both the netpol view and the AuthorizationPolicy
view individually; only the fused verdict sees it. Mid-migration windows are when
operators trust tooling least — catching the one failure class unique to that window
is disproportionate trust-building.

### 2.3 Ambient interop checks, cluster-wide

**What:** `meshistio.ValidateExternalRules` already exists but only fires node-scoped
on click, as bare strings (`internal/api/node-data.go:44`). Loop workloads, emit as a
typed issue. Code is written; near-zero cost.

**Why it matters:** a NetworkPolicy that blocks HBONE port 15008 breaks ambient
dataplane traffic silently — the policy looks correct at L3, the mesh looks healthy,
and traffic still dies. Nobody audits per-node by clicking every workload; a
cluster-wide sweep is the only way this gets seen before it bites. Highest
value-per-line-of-code item on this list.

### 2.4 Policy-does-nothing

**What:** O(rules) scan for `CoverageUnenforced`/`CoverageAudit`
(`internal/models/rule.go:33-34` — already stamped on rules).

**Why it matters:** an operator who writes a policy naming a direction that constrains
nothing believes the workload is protected. It isn't. This is a *false sense of
security* bug — worse than a visible failure because nothing ever pages; the gap is
discovered during the incident it should have prevented. Detection is trivial because
the coverage classification already exists.

### 2.5 Selector-matches-nothing (needs verification first)

**What:** policy whose pod selector matches zero pods (typo'd label, renamed app).

**Why it matters:** same false-security class as 2.4 — the policy is deployed, reviewed,
and inert. Extremely common after label refactors.

**Blocker:** unverified whether engines retain non-matching policies after compile —
if they're dropped during `Evaluate`, this needs an engine change (medium cost).
Check the `getPolicies` → compile path before pricing.

### Deferred: port-mismatch detection

"Allow to the right peer on the wrong port" — belongs on this list eventually but is
blocked on the port-blind verdict fix (4.2). Detecting port mismatches with a
port-blind prober would just generate noise.

---

## 3. Workflow features on top (ranked)

### 3.1 Stable issue identity — prerequisite for everything below

**What:** deterministic key per issue — hash of (type, engine, srcID, dstID, culprit
set).

**Why it matters:** without identity, an issue is just a row that exists this refresh.
Delta, export references, ack/suppress, deep links — every workflow feature needs to
say "*this* issue, the same one as before". Retrofitting identity after workflows
exist means migrating whatever consumers accumulated. Cheap now, expensive later.

### 3.2 Delta between refreshes

**What:** "new since last load" marker. Needs 3.1 + previous snapshot held in server
memory. No persistence layer.

**Why it matters:** this is the rollout-review killer feature. The operator's actual
question after applying a policy is never "what issues exist" — it's "what did *my
change* break". Diff answers it in one glance; inventory forces a manual before/after
comparison the operator won't do reliably at 40 rows. This single feature converts
the drawer from a one-time audit report into a tool used on every change.

### 3.3 Export (CSV / markdown)

**What:** dump the drawer contents.

**Why it matters:** the real workflow is pasting findings into a change-review PR or a
ticket. Without export, operators screenshot — which loses the culprit refs and
can't be grepped later. Cheap to build, removes the friction between "tool found it"
and "team acted on it".

### 3.4 Suppress / acknowledge — premature

Needs persistence, identity (3.1), and evidence that real clusters produce noise the
layering demotion doesn't already absorb. Ship 3.1–3.3, run against a real cluster,
then decide. Building suppression before knowing the noise profile risks building the
wrong suppression model (per-issue? per-pair? per-policy?).

### 3.5 Notifications — don't build

Pull-based tool computing over a request-time cache. Alerting from stale cache =
paging people on data from the last time someone opened a browser tab. Off the table
until there's a watch/informer refresh loop (see `docs/informers.md`). Same gate
applies to moving severity server-side: UI owns the mapping today, which is fine —
the moment external consumers appear, severity must move into the API or every
consumer reinvents it differently.

---

## 4. Must-change in current implementation (ranked)

### 4.1 `/api/issues` quietly lies about scope and freshness

Scans whatever previous `/api/graph` calls left in cache
(`internal/api/node-data.go:76-80`) and returns a bare array — no namespaces-scanned,
no evaluated-at. Operator reads "3 issues" as "cluster has 3 issues"; truth is "3
issues among browsed namespaces, as of some earlier point".

**Why fix now:** trust is the product — a scope lie here poisons every detector built
on top. And the bare array is an API-shape trap: metadata can't be added later
without breaking every consumer. Fix while there are zero clients:
`{issues, scannedNamespaces, evaluatedAt}`.

### 4.2 Port-blind verdicts → false DNS negatives and false conflicts

`collectToSide` classifies an allow by peer ID only, ports ignored
(`internal/store/reachability.go:20-31`). An egress allow to kube-dns on 9153/metrics
counts as DNS-permitted while UDP/53 stays blocked — the detector says fine,
resolution is down.

**Why it matters:** a false *negative* from a conflict detector is the worst failure
mode a trust product has — the operator checked, the tool said clean, the outage
happened anyway. One such miss and the drawer is never trusted again. Same blindness
makes port-scoped denies read as full explicit-deny → false-positive conflicts (noise
that erodes trust from the other direction). Fix: optional port on the probe; DNS
detector probes 53.

### 4.3 DNS check is half a verdict

`MissingDns` runs `collectToSide` only (`internal/store/buildIssues.go:65`) — src
egress side. An ingress default-deny on the kube-dns namespace breaks DNS identically
and isn't caught. The full probe (`IsNodesReachable`) exists; use it.

Also `matches[0]` at `buildIssues.go:43`: with multiple DNS workloads
(node-local-dns), map-order pick means issues flap between refreshes — an issue that
appears and disappears without any cluster change destroys confidence faster than a
missing detector. Check all matches; flag only when all are blocked.

### 4.4 Perf: per-probe index rebuild at target scale

`buildWorkloadIDIndex` is rebuilt inside every probe
(`internal/store/reachability.go:165`); `PolicyIssues` sweeps all allow rules → at
tens of namespaces / thousands of pods this is O(pairs × workloads) allocations per
request, held under `s.mu.RLock` while `handleGraph` waits on the write lock — the
issues sweep can visibly stall graph loads. Build once in `newReachabilityProber`,
thread through. Same for the linear `IdtoWorkload` scans per rule
(`buildIssues.go:107,117`, `internal/store/layering.go:139,142`) — use the index.

### 4.5 Three advertised issue types never fire

`mesh policy`, `mesh conflict`, `node lockout` exist in the UI type union, severity
map, and filter chips (`ui/src/data/policies.ts:244-249`) but no detector emits them
(`GetIssues` runs two detectors, `internal/store/buildIssues.go:16-23`).

**Why it matters:** an operator filters to "node lockout", sees empty, concludes the
cluster is clean. A filter for a detector that doesn't exist is a silent lie. Either
wire 2.1/2.2 or pull the types from the UI list until they exist.

### 4.6 Minor

- `Issue` embeds full `*WorkloadNode` src/dst (`internal/models/issues.go:27-29`) —
  DNS issues emit one per workload per engine, each carrying full labels/statuses.
  ID + label suffice; the UI already holds the node list. Payload bloat at scale.
- `checkedPolicy` dedup key `srcID+"-"+dstID` (`buildIssues.go:101`) collides when
  IDs contain dashes (they do — ns node ids). Use a struct key.

---

## Order of operations

1. **4.1 + 4.2 first** — new detectors built on a scope-lying endpoint and port-blind
   verdicts multiply the lie.
2. 4.3–4.5 alongside the first new detector (2.1 node lockout wires the dead type).
3. 2.2 / 2.3 next (mesh detectors — plumbing exists).
4. 3.1 identity → 3.2 delta → 3.3 export, in that order.
5. Re-evaluate 3.4 suppression after real-cluster mileage; 3.5 notifications gated on
   informers.
