# Changelog

## [Unreleased]

### Features

* **Istio AuthorizationPolicy support (L3/L4)** — ALLOW and DENY rules rendered as engine-attributed edges. namespaces, ipBlocks, and ports all participate in the graph.
* **Multi-engine policy graph** — k8s `NetworkPolicy` and Istio `AuthorizationPolicy` rendered side-by-side. Each edge carries its source engine; UI gains a policy-source filter to view one engine at a time.
* **Per-engine + effective status badges** — workload detail panel now shows what each engine's policies intend, plus the AND'd effective posture across engines. Cross-engine intersection respects each engine's "doesn't constrain this dimension" semantics.
* **L7 matcher surfacing** — Istio operation blocks (`hosts`, `methods`, `paths`) attached to edges. New `L7` badge marks workloads gated by L7 rules.
* **Action-aware edges** — ALLOW vs DENY rules render distinctly so deny posture is visible at a glance.
* **`/api/cluster-state` enumerates policy sources** — UI populates the policy-engine filter chips before the graph loads.

### Fixes

* Engine fetch errors are surfaced to `/api/graph` instead of producing a silent empty graph.
* `PolicySource` interface now returns `(EvaluationResult, error)` — engine failures fail fast.

---

## [0.0.1](http://gitlab.dev.local/home-lab/network-policy-visualizer/compare/0.0.0...0.0.1) (2026-05-08)

### Features

* **Live cluster graph** — pods, cronjobs, and `NetworkPolicy` objects rendered as a connected graph.
* **Workload status badges** marking effective security posture: `internet-full` / `internet-egress` / `internet-ingress`, `lan-*`, `api-server-egress`, `air-gapped`, `cross-namespace`, and `ns-full` / `ns-egress` / `ns-ingress` access.
* **Direction-aware edges** — ingress / egress / both, with port detail and protocol.
* **Namespace-level edge collapsing** — catch-all peers (empty `podSelector` / namespace-only selector) collapse to the namespace node.
* **Multi-policy bundle aggregation** — edges between the same source/target pair are grouped with a policy count.
* **Aggregate-by-namespace view** — collapse the graph to namespace nodes for tenant-isolation review.
* **Demo mode** — embedded test fixtures (`DEMO_MODE=true`) for running without a live cluster.
* **Single static binary** — embedded UI, no separate frontend deploy. Read-only on the cluster (`LIST` only).
