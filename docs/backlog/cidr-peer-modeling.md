# CIDR peer modeling — findings + plan

## Context

Backend accepts CIDR peers from `NetworkPolicy.spec.ingress[].from[].ipBlock` / `egress[].to[].ipBlock` but drops most information:

- `IPBlock.CIDR` — stored raw as `Rule.DstID` (bare string, no typing).
- `IPBlock.Except[]` — read nowhere. Silently discarded.
- No CIDR node ever emitted → edges dangle at nonexistent node IDs.
- Istio engine ignores `IpBlocks[]` entirely at the rule level (`istio/buildRules.go` `expandFromSource`) — **out of scope for this iteration** (no user demand yet).

Frontend must invent placeholder nodes or drop edges. Detail panel cannot show "allow 10.0.0.0/8 EXCEPT 10.1.2.0/24" because Except never leaves k8s API object.

Goal: make CIDR peers first-class in the graph — one node per unique CIDR (shared across policies and namespaces), Except entries surface as visible carve-out edges distinguishable from Istio DENY, node set is authoritative for peer type.

## Design decisions

1. **Graph shape** — per-CIDR nodes. One `WorkloadNode` per unique CIDR string across all rules. ID prefixed `cidr:` to guarantee no collision with k8s UIDs (which are GUIDs, but sharing an ID namespace with `cidr:0.0.0.0/0` etc. is a footgun).
2. **Except handling** — emit second `Rule` with `Action=ActionDeny`, `DstID=cidr:<exceptCIDR>`, same `Contributor` as parent allow rule. But mark as carve-out (see #3) so it does not read as a standalone deny.
3. **Except vs Istio DENY disambiguation** — Except-derived rules stamp `Coverage=CoverageExcept` (new enum value) AND keep `Contributor.Action="allow"` (they come from an allow policy). Rule-level `Action=ActionDeny` for edge grouping. Istio DENY policies keep `Contributor.Action="deny"` + `Coverage` unchanged. Frontend distinguishes by `Coverage=CoverageExcept`: render as hatched cutout of parent allow, not a red standalone deny.
4. **Peer typing** — drop the `PeerKind` sibling-field idea. Node set is authoritative: given an endpoint ID, look up the `WorkloadNode` and read `.Type`. Requires engine-time node synthesis (#5) so the node always exists before edges get grouped.
5. **CIDR node synthesis at engine time** — engine is the only layer that knows a peer is a CIDR. Add `Nodes []WorkloadNode` to `EvaluationResult`. `BuildGraph` concatenates + dedups by ID. Removes post-render synthesis footgun (`renderEngine.go` no longer sees rules whose endpoints aren't in the node set) and gives Istio parity a natural landing spot when it ships.
6. **Status keys — explicit defer.** Except changes classification math (`0.0.0.0/0` except `169.254.169.254/32` still Internet; `10.0.0.0/8` except `10.0.0.0/8` no longer LAN). Set arithmetic non-trivial. This PR does NOT touch status-key derivation — flag in code comment + docs: "Except renders as Deny edge but does not yet affect status keys."

## Current state — file:line map

| Concern | Location | State |
|---|---|---|
| CIDR read | `internal/policy/k8spolicy/buildRules.go:271-273` | `dstIDs = append(dstIDs, peer.IPBlock.CIDR)` — bare string, no kind flag |
| Except read | anywhere | Never referenced. `grep -r "IPBlock.Except\|\.Except\[" internal/` = zero hits |
| Rule model | `internal/models/rule.go:5-16` | `SrcID`/`DstID string`; already has `Action RuleAction` (Allow/Deny) — Deny path exists |
| Coverage enum | `internal/models/rule.go:27-35` | Has DenyAll / AllowAll / AllowAllNs / Restricted / Unenforced / Audit — no Except value |
| NodeType enum | `internal/models/node.go:16-23` | `NodeTypeExternal="external"` exists, never instantiated. No `NodeTypeCIDR` |
| Node synthesis | `internal/graph/buildNodes.go` `AssembleNsIndex` | Only pods, cronjobs, namespace nodes |
| Edge render | `internal/graph/renderEngine.go:32-56` | Groups on `(srcID, dstID, ...)`. CIDR string used as-is; no node lookup / validation |
| Status keys | `internal/policy/k8spolicy/buildStatusKeys.go:126-156` | Classifies IPBlock via helpers in `policy/networkPolicyHelpers.go` — Except still not consulted (deferred, see #6) |
| Istio parity | `internal/policy/istio/buildRules.go` `expandFromSource` | `IpBlocks[]` skipped — out of scope this PR |

## Implementation plan

### 1. Coverage: add `CoverageExcept`

`internal/models/rule.go` — extend `Coverage` enum:

```go
CoverageExcept Coverage = "except"
```

Frontend switches on this to render carve-out styling (hatched cutout) instead of standalone deny red.

### 2. CIDR node type

`internal/models/node.go`

Add:
```go
NodeTypeCIDR NodeType = "cidr"
```

Decision on existing `NodeTypeExternal` (never instantiated, `node.go:20`): **delete it**. Dead code. If a non-CIDR external actor lands later (FQDN, ServiceEntry), add it then with clear semantics. Two dead-adjacent types is worse than one.

### 3. CIDR + Except emission — k8s engine

`internal/policy/k8spolicy/buildRules.go` — modify `expandPeerRules` (lines 270-273):

Current:
```go
if peer.IPBlock != nil {
    dstIDs = append(dstIDs, peer.IPBlock.CIDR)
}
```

Replace with logic that:
- Emits one Allow rule: `Action=ActionAllow`, `DstID="cidr:"+peer.IPBlock.CIDR`, `Contributor.Action="allow"`, `Coverage` inherited from surrounding context.
- For each entry in `peer.IPBlock.Except`, emits a second Rule: `Action=ActionDeny`, `DstID="cidr:"+exceptCIDR`, same `Contributor` (including `Action="allow"` — the source policy is still an allow), `Coverage=CoverageExcept`.

Ingress path (`expandIngressRules`) — `buildAllowRulesByNs:41-42` swaps `SrcID`/`DstID`. Except deny rules on ingress land with `SrcID="cidr:..."` after the swap; existing swap logic is direction-agnostic on the string field so no change needed there. Selectors stay correct — verified against `expandIngressRules:198-204`.

Set `SrcID` correctly on allow-all / deny-all fallback rules in `expandEgressRules:121` and `expandIngressRules:172` — these already point at workload/ns nodes, no CIDR involved.

### 4. Engine-time CIDR node synthesis

`internal/models/evaluation.go` — extend `EvaluationResult`:

```go
type EvaluationResult struct {
    AllowByNs      map[string][]Rule
    DenyByNs       map[string][]Rule
    PolicyStatuses map[string]PolicyStatus
    Nodes          []WorkloadNode // synthetic nodes (currently: CIDR nodes)
}
```

`internal/policy/k8spolicy/evaluate.go` — after `buildAllowRulesByNs` returns, walk emitted rules, collect every `DstID` with prefix `cidr:`, dedup, emit `WorkloadNode{ID, Label: strings.TrimPrefix(id, "cidr:"), Type: NodeTypeCIDR, Namespace: ""}` per unique CIDR. Attach to `EvaluationResult.Nodes`.

`internal/graph/buildGraph.go` — after loop over `cache.EvaluationResults`, dedup + concat `result.Nodes` into `allNodes`. Dedup by ID so cross-engine CIDR references (when Istio parity ships) collapse to one node.

CIDR nodes have `Namespace: ""` — Cytoscape compound-parent grouping does not attach them to any ns, which is correct (CIDRs are cluster-external / cross-ns).

### 5. Edge rendering — no changes to grouping key

`renderEngine.go:32-56` grouping key already includes `dstID`. Multiple Except carve-outs on the same parent allow produce multiple edges (different `dstID`) — correct: one visible edge per CIDR. `PolicyEdge` gets no new fields (`PeerKind` dropped per #4 of decisions). `Coverage` already on `PolicyEdge` (`graph.go:19`), so `CoverageExcept` propagates automatically via `edge.Coverage = rule.Coverage` at `renderEngine.go:53`.

## Files touched

- `internal/models/rule.go` — add `CoverageExcept`
- `internal/models/node.go` — add `NodeTypeCIDR`, delete `NodeTypeExternal`
- `internal/models/evaluation.go` — add `EvaluationResult.Nodes`
- `internal/policy/k8spolicy/buildRules.go` — prefix CIDR IDs, emit Except deny rules, stamp `Coverage=CoverageExcept`
- `internal/policy/k8spolicy/evaluate.go` — synthesize CIDR `Nodes` into `EvaluationResult`
- `internal/graph/buildGraph.go` — concatenate + dedup `EvaluationResult.Nodes` into `allNodes`
- `internal/policy/k8spolicy/buildStatusKeys.go` — no change (see #6); add comment noting Except deferral

## Out of scope (follow-up tickets)

- **Istio CIDR parity** — `istio/buildRules.go` `expandFromSource` still ignores `IpBlocks[]` / `NotIpBlocks[]`. No user demand. Note in code comment so it does not silently rot.
- **Status-key impact of Except** — `(cidr - union(except))` classification. Non-trivial CIDR set arithmetic. Ticket separately.
- **Frontend styling** — Cytoscape node style for `type=cidr`; carve-out (`Coverage=CoverageExcept`) render distinct from standalone Istio DENY. Detail panel: show CIDR classification via `IsIpBlockInternetAccess` etc. Confirm scope with user before starting.

## Verification

### Unit tests

- `internal/policy/k8spolicy/edge_test.go` — new tests:
  - `TestBuildAllowTuples_IPBlockCIDR_PrefixedID` — CIDR peer produces rule with `DstID="cidr:10.0.0.0/8"`.
  - `TestBuildAllowTuples_IPBlockExcept` — `ipBlock: {cidr: 10.0.0.0/8, except: [10.1.0.0/16, 10.2.0.0/16]}` produces 1 Allow + 2 Deny rules, all `DstID` prefixed `cidr:`, Contributor identical, Deny rules stamp `Coverage=CoverageExcept`.
  - `TestBuildAllowTuples_IPBlockExcept_SelfNullifying` — `ipBlock: {cidr: 10.0.0.0/8, except: [10.0.0.0/8]}` produces Allow + Deny on same CIDR (pathological but legal — operator should see policy does nothing).
  - `TestBuildAllowTuples_IngressCIDRPeer_SrcIDPrefixed` — ingress path swap lands CIDR in `SrcID` with prefix intact.
- `internal/policy/k8spolicy/evaluate_test.go` (new or extend) — `TestEvaluate_EmitsCIDRNodes`: single policy referencing `10.0.0.0/8` + `10.1.0.0/16` produces `EvaluationResult.Nodes` with two entries, both `Type=NodeTypeCIDR`, `Namespace=""`, IDs prefixed.
- `internal/graph/buildGraph_test.go` — `TestBuildGraph_DedupsCIDRNodesAcrossNamespaces`: two ns each referencing `10.0.0.0/8` → one node in final graph. Also `TestBuildGraph_CIDRNodeSurvivesPartialCacheInvalidation`: cache has ns A + ns B both referencing same CIDR; drop ns A; node survives.
- `internal/graph/render_edges_test.go` — CIDR Except rule renders edge with `Coverage=CoverageExcept, Action=ActionDeny`.

### Live smoke test

- `DEMO_MODE=true go run main.go`, hit `/api/graph`.
- Confirm response includes node `{"id":"cidr:0.0.0.0/0","type":"cidr","namespace":""}` and edge with `targetKind` absent but `target` pointing at said node.
- If a demo fixture has Except: confirm Deny edge appears with `coverage: "except"`.

### Regression

- `go test ./...` — no failures.
