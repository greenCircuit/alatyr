# ADR 0001: Generic Policy-Source Abstraction

**Status:** Accepted
**Date:** 2026-05-19
**Branch:** `2-add-istio-l3-l4-authorization-policy-support`

## Context

The visualizer needs to render Istio `AuthorizationPolicy` alongside K8s `NetworkPolicy`. The user stated future addons will include Calico and Cilium and explicitly asked the backend to be "generic enough" so each new source doesn't require an engine rewrite.

Today, edge construction lives in `internal/graph/build-edges.go` and is hard-coded to K8s NP. Adding Istio directly into that file would mean a second, similarly-shaped rewrite when Calico lands.

## Decision

Introduce a `PolicySource` interface in a new `internal/policy/` package. Every policy source (including K8s NP) implements it. A generic `intersect.go` consumes `[]PolicySource` and produces effective allow tuples without referencing any specific source.

K8s NP code is **retrofitted into the interface in this branch**, not deferred. See alternatives below.

## Alternatives considered

**A. Add Istio code directly to `internal/graph/` alongside K8s NP. Refactor when Calico lands.**

Smaller PR now. Rejected because:
- Same refactor would happen 4-6 months out with more code on top.
- Intersection logic would carry a permanent K8s special case.
- User explicitly asked for generic backend up front.

**B. Generic interface, but leave K8s NP out of it for now. Istio is the first implementation; K8s gets retrofitted "later".**

Half-measure. Rejected because the intersection engine would need a hard-coded "always also AND K8s edges" branch — defeats the abstraction's purpose. Calico would then need the same special-case treatment for K8s before being added.

**C. Generic interface, retrofit K8s now (chosen).**

Larger first PR but lands the abstraction in one coherent piece. Adding Istio becomes a pure addition with no engine edits.

## Consequences

**Positive:**
- New sources are additive — implement the interface, `init() { Register(&src{}) }`, allowlist the name in the API.
- Intersection engine is testable with a fake source double; no Kubernetes or Istio clients required for engine tests.
- Status keys become source-namespaced (`istio.AuthPolicyDenyAll`, `k8s.EgressUnmatchedByIstio`) — clear ownership.

**Negative:**
- First PR is larger because K8s NP code moves packages. Existing tests in `internal/graph/edge_test.go` and `nodes_test.go` need their imports updated. Behavior must not change — addressed via test-first migration in PR 2a.
- Two layers of indirection (source → tuples → edges) versus today's direct policy → edge mapping. Justified by the engine being shared across N sources.

**Migration risk:**
- Existing K8s NP behavior must be preserved bit-for-bit. PR 2a does no semantic change — only relocates code and introduces the trivial `N=1` intersection path.
