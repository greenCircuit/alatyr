# Neighbor grouping in the DetailPanel — UX review + backlog

Status of the side-panel neighbor list (workload click → inbound/outbound peers).
Records what shipped and the frontend-ux review findings still open, so we can
pick them up later.

## Problem being solved

Backend emits one `NeighborRef` per (peer, rule). A single ingress policy that
admits N sources fans out to N refs sharing the same policy/direction/ports.
Old panel repeated the invariant fields (Policy, Direction, Ports, Destination)
once per peer → bloat.

## Shipped

- `shared/groupNeighborsByPolicy.ts` — folds a direction's refs by
  `(policy identity, direction, action, ports, L7)` into per-policy groups.
  Contributor-less refs get a per-ref key (never merge under a policy header).
- `shared/rows.tsx` — `NeighborGroup` (shared header + peer sublist),
  `NeighborPeerRow` (lean peer line), `NeighborSection`, `NeighborList`.
- **Deny-first sections**: `NeighborList` renders `⛔ Denied by policy` above
  `✓ Allowed by policy`, mirroring eval order (deny wins). Section labels only
  when denies exist; allow-only (k8s) renders flat. Deny cards get a danger
  border.
- Consumed in `views/WorkloadView.tsx` (inbound + outbound).
- Tests: `groupNeighborsByPolicy.test.ts`.
- **YAML highlight correlation**: grouping had collapsed per-rule highlights into
  one card button, lighting every peer the same color (no rule↔peer correlation).
  Fixed with a hybrid on the existing single-color mechanism — header button
  highlights all selectors (overview), each `NeighborPeerRow` gets its own YAML
  button highlighting only that peer's selector + this-node. One active highlight
  at a time keeps single-color correct. Rejected multi-color+legend (fights the
  accordion, doesn't scale past ~4 colorblind-safe hues) and selection-driven
  co-highlight (more store plumbing) per frontend-ux review. Density accepted;
  accordion hides per-peer buttons for big fan-outs.
- **Two-tone src/dst highlight**: YAML highlight split into node-side (amber,
  `.yamlHighlight`) vs peer-side (teal, `.yamlHighlightPeer`). Two fixed colors
  regardless of peer count — colorblind-safe, no legend. Threaded `highlightPeer`
  through `ManifestButton` → `manifestStore` target → `ManifestModal` two-tier
  `renderYaml` (peer wins on shared lines). Applied to header, per-peer, and
  outbound `RuleRow` buttons.

## Open — from frontend-ux review, prioritized

### 1. Per-peer cross-mark for overridden allows (trust-critical) — DEFERRED
Deny/allow sections separate the *cards*, but a peer allowed by policy X **and**
denied by policy Y still appears in both with no link. Operator reading the allow
card can miss the deny. Partial coverage shipped: deny section above allow, and
the same peer surfaces in both the Denied and Allowed chip rows.

Decision: **not** fixing via frontend cross-marking. Effective reachability
(allow/deny precedence per neighbor) will be resolved server-side by a new
node→neighbor reachability endpoint; the panel will render the backend verdict
instead of inferring override from grouped rules. Frontend stays a dumb renderer;
no allow/deny correlation logic in the UI. Revisit when that endpoint lands.

### 2. Card headline is a count; policy name buried — DONE
Shipped: policy name promoted to card title (bold), peer count demoted to a small
subtitle, manifest button + deny badge lifted to the header row. Added a cross-ns
badge (`ns: <namespace>`, warning) shown when the policy's namespace differs from
the clicked workload's — flags cross-ns ingress that's otherwise easy to miss.
`node.namespace` threaded through `NeighborList` → `NeighborSection` →
`NeighborGroup`.

### 3 + 4. Double-count + multi-policy inflation — DONE (summary layer)
Shipped: `distinctPeers()` in `groupNeighborsByPolicy.ts` + `PeerChips` row per
section in `rows.tsx`. Each section title carries the **distinct-peer count**
(not ref count), and a chip row lists each peer once above the per-policy cards.
Policy cards kept intact by design — they show *how* each policy selects peers
(the "how"); the chip row answers "who". A peer admitted by several policies
counts once in chips, recurs in the cards (intended). A peer in both Denied +
Allowed chip rows surfaces the conflict — partial coverage of #1.

Parent header now fixed too: `WorkloadView` `Inbound`/`Outbound` counts use
`distinctPeerCount()` — distinct peers, not refs. Header, chip row, and section
counts all share one denominator. Thread closed.

### 5. `_no_policy` fallback comment is wrong — DONE
Fixed the comment (CIDR peers group normally; blank is `Workload`, not
contributor). Confirmed backend sends `Contributor PolicyRef` as a value type
(`models/rule.go:10`), never null → the old `!contributor` guard was dead code
and a zero-value contributor would have formed an empty-named merged card.
Guard tightened to `!contributor?.name`; render side treats empty-name
contributor as absent. Defensive — doesn't occur with today's engines.

Original finding:
`groupNeighborsByPolicy.ts` drops to a per-ref key only when `rule.contributor`
is nil, and the comment calls that "raw CIDR / unresolved". Wrong — a CIDR peer
still has a contributor policy; what's blank is `NeighborRef.Workload` (peer
resolution), and the peer row already falls back to the raw id. The `_no_policy`
path fires only for genuinely unattributed rules. Fix the comment. Confirm with
backend whether `contributor` is ever actually nil — if never, the fallback is
dead-ish code.

### 6. Scan-ability: sort + collapse — DONE
Shipped: `NeighborGroup` peer list is an accordion (`▾/▸ To (N)` toggle).
Default collapsed above `PEER_COLLAPSE_THRESHOLD` (5), open below. Peers sorted
ns→name via `peerSortKey`. Chip row covers the overview when a card is collapsed.

Follow-up if wanted: whole-direction (Outbound/Inbound) or per-section
(Denied/Allowed) collapse — not done; only per-card peer lists collapse today.

## Minor
- Direction badge inside every group is near-constant per per-engine section
  (always ingress-side inbound). Low signal — consider lifting to section header.
- `key={i}` on peer rows is fine for static lists; wants a stable peer id once
  sort (#6) lands.
- Engine badge correctly inherited from the parent per-engine card in
  `WorkloadView` — not needed on the group.

One fork to decide

Section-header count: distinct peers ("who can reach me") or flow/ref count ("how many rules touch me")? Decides whether #3+#4 are one fix or two.

My rec: distinct peers. Matches operator's actual question, kills the double-count and the multi-policy inflation in one move.

Want me to implement #1 (deny precedence) + #2/#3 (policy headline + distinct-peer count)? Those three are the trust-critical set. #5 comment fix trivial, fold in. #6 collapse/sort is polish â separate pass.