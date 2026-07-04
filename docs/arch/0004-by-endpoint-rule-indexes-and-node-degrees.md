# ADR 0004: By-Endpoint Rule Indexes and Node-Degrees Endpoint

**Status:** Accepted
**Date:** 2026-06-29
**Branch:** `27-cross-ns-outbound-rules-not-recorded`

## Context

`store.GetNodeData(data, nodeId, ns)` (`internal/store/node-info.go`) returns a
node's outbound rules by scanning only `engineEvaluate.AllowByNs[ns]` /
`DenyByNs[ns]` — the node's **own** namespace bucket.

`*ByNs` maps are keyed by the **policy's** namespace
(`internal/policy/k8spolicy/buildRules.go:12-14`), and ingress rules flip their
endpoints during expansion (`buildRules.go:26-29`: `SrcID=peer`,
`DstID=selectedPod`). Net effect: an ingress policy in namespace B that allows
traffic **from** node A produces a rule `SrcID=A` bucketed under namespace B.
Clicking A scans only `AllowByNs[nsA]` and silently drops every cross-namespace
outbound neighbor. The same class hits Istio — `AuthorizationPolicy` is
ingress-only and lives in the destination namespace
(`internal/policy/istio/buildRules.go:27-32`). Egress is unaffected: a k8s
egress policy lives in the source namespace and is never flipped.

This is silent data loss in a tool whose product is trust — the panel shows a
partial neighbor set with no indication anything is missing.

A second need landed alongside the fix: a **table view** that lists workloads
with their neighbor counts (in/out degree) per engine, rows clickable for full
detail. The table renders rows from the graph nodes already in the frontend
store, then enriches them lazily.

## Decision

### 1. By-endpoint rule indexes on `EvaluationResult`

Add per-engine maps to `models.EvaluationResult`, keyed by real endpoint id
instead of policy namespace:

```go
AllowBySrc map[string][]Rule  // key = SrcID (outbound)
DenyBySrc  map[string][]Rule
AllowByDst map[string][]Rule  // key = DstID (inbound)
DenyByDst  map[string][]Rule
```

Built in `store` at cache-populate by walking the existing `*ByNs` buckets and
re-keying. **Engines stay untouched** — `PolicySource.Evaluate` does not change.
Re-keying is lossless: `*ByNs` holds the complete per-engine rule set, no
pipeline transform is namespace-dependent, so re-bucketing by `SrcID`/`DstID`
reconstructs the full edge set on real endpoints regardless of which policy-ns
bucket a rule landed in. The cross-namespace blind spot dissolves by
construction.

### 2. Fix `GetNodeData`

Read `*BySrc[nodeId]` / `*ByDst[nodeId]` instead of `*ByNs[ns]`. Strict superset
of today's behavior — same node, now with cross-namespace neighbors included.
Serves the click-detail path behind the existing `/api/node-info` endpoint for
both graph view and table view.

### 3. New endpoint `GET /api/node-degrees`

Returns the lightweight aggregate that drives the table:

```
map[nodeId]map[engine]NodeNeighbors
```

```go
type NeighborRef struct {
    ID        string `json:"id"`
    Namespace string `json:"namespace"`
}
type NodeNeighbors struct {
    Out []NeighborRef `json:"out"` // node is src
    In  []NeighborRef `json:"in"`  // node is dst
}
```

Distinct peers per direction per engine (dedup by `ID`). Count is `len` — no
separate count field. Namespace resolved via `buildWorkloadIDIndex` / `NsIndex`.
Neighbor ids + namespaces let the frontend resolve full info (label, type,
status) from the graph nodes it already holds.

### 4. Frontend load paths

- **Graph view** — loads `/api/graph`; node click hits `/api/node-info`.
- **Table view** — renders rows from graph nodes already in the zustand store,
  then lazily fetches `/api/node-degrees` and merges by `nodeId`; row click hits
  `/api/node-info`.

Counts come from the backend aggregate, not from frontend edge grouping: graph
edges are filtered and merged, so deriving degree from them would reproduce the
same blind spot and shift with active filters. The endpoint gives authoritative,
unfiltered totals.

## Alternatives considered

**A. Loop all `*ByNs` buckets inside `GetNodeData`.**
One-line correct fix. Rejected as the primary path because the table renders
degree for **all** nodes — a full-scan per node is O(nodes × rules) per table
load. Acceptable only if detail stayed click-only and lazy.

**B. Stamp degree onto graph nodes in the `/api/graph` payload.**
Rejected: table view does not necessarily load the graph; degree must be
fetchable independently. Riding the graph payload couples the two.

**C. Derive counts on the frontend from edges in the store.**
Rejected: edges are filtered/merged, so counts would be wrong cross-namespace
and would drift with filter state. The fix exists precisely because the edge
view loses cross-ns adjacency.

**D. Reuse `PolicyRef` for the neighbor list.**
Rejected: `PolicyRef` is the policy axis (`Source`/`Name`/`Namespace`/
`RuleIndex`), not the peer-node axis. It carries no `SrcID`/`DstID`.
`NeighborRef` is a distinct 2-field type. `PolicyRef` stays in
`NodeInfo.Policies` for the selects-this-node list.

**E. Collapse 4 maps to 2 (`Rule.Action` already distinguishes allow/deny).**
Viable simplification — halves the extra footprint, `GetNodeData` appends one
slice instead of two. Deferred; kept 4 maps to mirror the existing
`AllowByNs`/`DenyByNs` split and minimize churn. Revisit if footprint matters.

## Consequences

**Positive:**
- Cross-namespace outbound neighbors are correct by construction; no per-node
  full scan.
- Per-engine shape falls out for free — `Cache.EvaluationResults` is already
  `map[engine]EvaluationResult`; neighbors render grouped by engine like the
  current detail panel.
- Table degree and click detail share one source of truth (the by-endpoint
  indexes); no second code path to drift.

**Negative / footguns:**
- ~3× the rule **headers** in memory (by-src + by-dst on top of by-ns). `Rule`
  fields are reference types, so a value copy duplicates only the ~150–200B
  struct header, not `Ports`/selectors/`L7Match` backing. Low tens of MB at
  target scale (tens of namespaces, hundreds-to-thousands of pods). Copy, not
  pointer-share — pointer slices save little and add an aliasing footgun if a
  `*ByNs` slice is ever re-grown post-build.
- **Mixed node kinds in neighbor sets.** A `namespaceSelector`-only peer yields
  a namespace node id `ns-<name>` (`buildRules.go:108`,
  `istio/buildRules.go:169-170`), not pod ids — and `ns-<name>` is in
  `buildWorkloadIDIndex` (`buildNodes.go:63`), so it resolves. Dedup-by-id is
  correct, but an edge to `ns-foo` and an edge to a pod inside `foo` count as
  two distinct neighbors. Label the table column "policy neighbors", not "pods
  it can reach" — the latter would be a lie.
- **allow+deny to the same peer collapses to one `NeighborRef`.** `NeighborRef`
  carries no action, so this table is adjacency, not a verdict. The view must
  not imply "allowed". Per-rule allow/deny detail stays in `/api/node-info`. If
  allow-vs-deny is ever needed in the table, the info is thrown away at dedup —
  decide before shipping.
- **Do not gate neighbor inclusion on namespace resolution.** CIDR/external ids
  (`buildRules.go:150`) never resolve via the index → `Namespace=""`. Mirror the
  existing unresolved-id swallow pattern (`buildStore.go:161`: keep the rule,
  blank the label): keep the neighbor, blank the namespace. Gating on resolution
  success silently drops external edges from the count.

**Explicitly out of scope:**
- `IsNodesReachable` (`node-info.go:62-101`) stays on `*ByNs`. It takes both src
  and dst namespaces explicitly and scans egress-from-srcNs + ingress-from-dstNs
  — correct for a known pair, no blind spot. Do not consistency-refactor it onto
  the new maps; no gain, risk to a working path.
- The stale comment at `node-info.go:9-11` ("inbound deliberately excluded")
  must be updated — inbound is now exposed via `node-degrees`.
- Frontend table merge-by-nodeId is unverified here (out of backend scope).
