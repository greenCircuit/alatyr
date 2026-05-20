# ADR 0002: Backend-Computed Intersection

**Status:** Accepted
**Date:** 2026-05-19
**Branch:** `2-add-istio-l3-l4-authorization-policy-support`

## Context

When the user selects "both" policy sources in the UI, the displayed graph must show traffic that is allowed by **all** selected sources (true runtime reachability). Two places this logic could live:

1. **Frontend.** Backend returns per-source edges tagged with source. UI does the AND.
2. **Backend.** Backend computes effective allow tuples per source, intersects them, returns a single merged edge set with per-source contributor refs.

## Decision

Backend computes the intersection. API returns one `[]PolicyEdge` per request, with each edge carrying a `contributors` map keyed by source name.

## Alternatives considered

**Frontend intersection (rejected).**

Reasons:
- Default semantics differ per source. K8s NP: workload selected by an NP → default-deny that direction; otherwise default-allow. Istio: workload targeted by an `ALLOW` policy → default-deny; otherwise default-allow; DENY policies subtract regardless. Encoding these defaults correctly in TypeScript across multiple sources is fragile.
- Granularity mismatch: K8s NPs commonly target specific pods, Istio AuthPolicies commonly target whole namespaces (user-confirmed convention). Intersecting requires expanding both to `(srcPodID, dstPodID, port)` tuples — repeating that logic in JS duplicates work and complicates tests.
- Multi-policy stacking: Istio aggregates all policies on a workload before applying (all DENYs, then any ALLOW). Cannot treat each policy as a standalone edge then intersect — gives wrong answers.
- Future sources (Calico, Cilium) add more default-semantics quirks. Centralizing in Go keeps the JS layer purely presentational.

**Hybrid (per-policy edges plus separate effective endpoint) (rejected).**

Discussed and dismissed in design. Doubles the API surface for marginal gain. Click-through to contributing policies is achievable from the merged edge's `contributors` map.

## Consequences

**Positive:**
- One source of truth for "what is actually allowed". Engine is unit-testable with a fake-source double.
- Frontend logic stays simple: render edges, render badges, list contributors on click.
- Status keys (`k8s.EgressUnmatchedByIstio` etc.) can be computed in the backend where all source data is colocated.

**Negative:**
- Backend cannot return raw per-source edges in the same response. If a future need arises ("show me edges allowed by K8s but blocked by Istio") it requires a separate endpoint or a flag. Acceptable — current product doesn't need it.
- Slightly more data per request (`contributors` map) than today's single `policyName` field. Negligible at expected graph sizes.

## Compatibility

`PolicyEdge.PolicyName` is retained and filled from the first K8s contributor when present. Existing UI code that reads `policyName` keeps working until updated to use `contributors`.
