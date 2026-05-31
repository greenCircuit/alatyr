# ADR 0002: Backend-Computed Intersection

**Status:** Accepted (mechanics revised after implementation — see "Revision" below)
**Date:** 2026-05-19 (decision), revised 2026-05-31
**Branch:** `2-add-istio-l3-l4-authorization-policy-support`

## Context

When the cluster has both K8s NetworkPolicy and Istio AuthorizationPolicy in
play, the displayed graph must convey true reachability — what traffic the
combined policy surface actually permits. Two places this logic could live:

1. **Frontend.** Backend returns per-engine data tagged with engine. UI does
   the cross-engine reduction.
2. **Backend.** Backend evaluates each engine, performs the AND across
   engines server-side, and returns a single coherent view.

## Decision

Backend computes the cross-engine intersection.

## Alternatives considered

**Frontend intersection (rejected).**

- Default semantics differ per engine. K8s NP: workload selected by an NP →
  default-deny that direction; otherwise default-allow. Istio: workload
  targeted by an `ALLOW` policy → default-deny; otherwise default-allow;
  DENY policies subtract regardless. Encoding these per-engine defaults
  in TypeScript across multiple sources is fragile.
- Multi-policy stacking inside one engine (Istio aggregates all DENYs then
  any ALLOW) cannot be replicated as a per-policy reduction on the client.
- Future engines (Calico, Cilium) add more quirks. Centralizing in Go keeps
  the frontend purely presentational.

## Revision: where the intersection lives

Original plan was edge-level intersection: produce one merged `[]PolicyEdge`
with a `Contributors` map keyed by source. **That approach was discarded
during implementation.** Two reasons:

1. Engines disagree on **granularity** (k8s pods vs Istio namespace
   selectors, L3/L4 vs L7). Collapsing both into one edge set loses signal
   the operator needs — "k8s allows this, Istio doesn't" reads as a
   missing edge, not as the policy disagreement it is.
2. Engines disagree on **what an edge means** (k8s allow tuple vs Istio
   ALLOW vs Istio DENY). One merged edge with a contributors map made the
   detail panel ambiguous: which engine's action wins, what does L7 add.

**As-built:** intersection happens at the **status-key** layer, not the
edge layer:

- `renderEdges` (`internal/graph/renderEngine.go`) keys edges by
  `policySource` — each engine renders its own edges (allow + deny), the
  frontend filters/colors by source.
- `policy.IntersectPolicyStatus` (`internal/policy/statusKeys.go`) takes
  per-engine `PolicyStatus` per workload, AND's the access fields, OR's
  the locks, ignores zero-status engines as transparent. Output drives
  `WorkloadNode.Statuses` (effective badges).
- `WorkloadNode.StatusesBySource` keeps the per-engine derived keys so the
  detail panel can show the breakdown.
- `IsNodesReachable` (`internal/store/buildStore.go`) — the explicit
  src→dst question — applies the same AND-across-engines rule on the
  verdict (`deny` from any engine blocks; transparent engines don't
  subtract).

## Consequences

**Positive:**
- Engines stay independent and pluggable. Adding Calico = implement
  `PolicySource`, register in `defaultSources`. No engine-aware code in
  the graph or frontend.
- Operators see both views: per-engine edges (what each layer decided)
  plus effective badges (what the workload actually experiences). Neither
  is a lossy summary of the other.
- Intersection logic is unit-tested in isolation
  (`policy/intersect_test.go`, `store/buildStore_test.go`) without
  needing fixture cluster state.

**Negative:**
- Two passes over per-engine data — once for edges, once for status keys —
  versus one merged structure. Negligible at expected graph sizes.
- "What changes if I drop Istio entirely?" needs a UI toggle to hide the
  engine's edges and recompute effective badges client-side. Acceptable —
  current UI handles this via the engine filter.

## Compatibility

`PolicyEdge.policySource` is the canonical engine tag; the original
plan's `Contributors` map was never shipped.
