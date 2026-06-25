# Changelog

## [0.2.2]
(2026-06-23)

### Fixes

* **DENY AuthorizationPolicies render as DENY again** — every Istio `AuthorizationPolicy` with `action: DENY` was silently rendering as a green ALLOW edge because the action field wasn't propagated into emitted rules. Operators reviewing deny posture were seeing the wrong-colored arrows for the policies they cared about most.
* **Multi-source / multi-operation AuthorizationPolicies now show every edge they grant or block** — a policy with several `from[]` source blocks (e.g., `from ns-a OR ns-b`) or several `to[]` operation blocks (e.g., `{port 80, host X}` and `{port 443, method Y}`) was silently keeping only the last block. Every source-spec block now contributes its own edge with the correct port↔L7 pairing.
* **`from`-only AuthorizationPolicies produce edges again** — the common "block traffic from ns-evil" shape (a `from:` clause with no `to:`) was emitting zero rules and disappearing from the graph entirely. These now render with explicit "all ports" / "all L7" signals so the deny intent is visible.
* **Workload detail panel — Ports column shows real values** — was rendering "undefined/" on every rule row after a backend port-field rename moved past the frontend type. Detail panel now decodes the array shape correctly and renders an "all ports" chip when a rule grants unrestricted port access.

### Features

* **Explicit "all ports" and "all L7" signals on rules and edges** — backend now stamps `allPorts` / `allL7` booleans whenever a policy block grants unrestricted access on either dimension. Resolves the long-standing ambiguity between "policy didn't say anything about ports" and "policy explicitly restricted ports to none" so the UI can render an honest "ALL" badge instead of leaving the chip row empty and ambiguous.

### Internals & cleanup

* Istio rule emission rewritten so each source-spec block (one `to[]` × one `from[]`) produces its own atomic `Rule`. Pairing between ports and L7 predicates is preserved by construction, removing the matrix lie at the aggregation layer.
* `models.Rule.Port` / `models.NodeRule.Port` renamed to `Ports` with matching JSON tags; `Rule.Contributor` JSON tag corrected from plural to singular. Frontend `NodeRule` interface and the detail-panel `RuleRow` updated to match.
* New unit tests pin the multi-`to[]`, multi-`from[]`, no-`to[]`, DENY-action, and AllL7-invariant contracts in `internal/policy/istio/istio_test.go` so future refactors can't silently regress them.
* New design doc `docs/backlog/l7-and-port-allowance-shape.md` records the discussion behind the allowance-list direction and the remaining cleanup list (port dedup keyed by number alone, L7 order-sensitivity, `Rule.Validate()` invariant check).

---

## [0.2.1]
(2026-06-13)

### Features

* **Istio ambient-mesh membership + mTLS resolution** — new `internal/mesh` abstraction with an Istio source that detects ambient enrollment from the `istio.io/dataplane-mode` label (workload-level wins over namespace) and resolves the effective mTLS posture by walking `PeerAuthentication` precedence (workload → ns → mesh root). Per-port overrides, UNSET fall-through, and same-scope tie-break by `creationTimestamp` are all honored.
* **Mesh section in the workload detail panel** — clicking a node now shows whether the workload is in mesh, which provider/mode, the effective mTLS verdict, every matching `PeerAuthentication` source, and a humanized issues list (root-ns selector ignored, duplicate at scope, port-level without selector, all-UNSET fall-back).
* **NetworkPolicy ⇄ ambient interop warnings** — when an ambient workload has NetworkPolicy ingress rules from other engines that restrict ports without permitting the ztunnel HBONE port (`15008`), the detail panel surfaces a warning explaining mesh traffic will be blocked and which port to add.
* **`MeshSource` interface + `CanReach`** — generic mesh-engine shape (`Name`, `Membership`, `ResolveMtls`, `CanReach`, `ValidateExternalRule`) so future providers plug in without graph-layer changes. `CanReach` denies src → dst only when the destination requires STRICT mTLS at the port and the source cannot speak mTLS (not enrolled or PA `DISABLE`).
* **`PeerAuthentication` in the k8s client + demo client** — real client wires the Istio security v1 informer; `DemoClient` serves PA fixtures so ambient/mTLS scenarios run without a live cluster.
* **Test fixtures** — `test-data/istioEngine/10-peer-authentications.yaml` covers strict/permissive/disable + port overrides + root-ns precedence; `11-ambient-netpol-interop.yaml` exercises the HBONE-port warning path.

### Internals & cleanup

* `internal/mesh/istio/` split into `istio.go` (package surface — consts, `source` type, ctor), `utils.go` (pure helpers — `convert`, `mtlsModeToScope`, `inAmbientMesh`, `effectiveMode`), `detect.go` (methods that hit the cluster), and `peerauth.go` (precedence walk). `ZtunnelHBONEPort = 15008` declared explicitly (the prior file didn't compile against an undeclared reference).
* Frontend `DetailPanel`, `FilterPanel`, and `PolicyGraph` each broken into `<Component>/index.tsx` + `parts/*` modules. Mesh rendering lives in `DetailPanel/parts/mesh.tsx`.
* New ADR `docs/arch/0003-istio-mesh-membership-and-mtls.md` documents the membership + mTLS resolution model.

---

## [0.2.0]
(2026-05-31)
5085574dc1277c7d86524ee3d9cde62391c20e49
### Features

* **Reachability checks between two workloads** — pin a source node, click any other node, get a side-by-side panel with the per-engine verdict (`allow` / `deny` / `not enforced`), the selecting policies on each end, every matched allow and deny rule, and which engine (if any) blocked the path. Multi-engine AND: traffic is reachable only when every engine permits it.
* **Per-engine breakdown in the workload detail panel** — clicking a single workload now shows what each engine's selecting policies say, in addition to the AND'd effective posture. Egress / ingress pills are colored to match the graph arrow directions.
* **Graph highlights during comparison** — the pinned source renders with a cyan ring, the destination with an amber ring; dimming of unrelated nodes is suppressed in compare mode so the canvas stays readable.
* **"Not enforced" engine state** — when an engine has no policy opinion about a pair (no locks, no matching rules) the panel labels it `not enforced` instead of leaving it ambiguous. Distinguishes "engine allowed it" from "engine never looked at it."
* **`/api/reachable`** — new endpoint that returns the structured `ReachabilityResult` powering the panel. Query params: `srcId`, `srcNs`, `dstId`, `dstNs`.
* **`/api/node-info`** — new endpoint returning per-engine rules + selecting policies for a single workload. Backs the detail-panel breakdown.
* **Backend cache layer** — `NSIndex` and per-engine `EvaluationResult` are cached on `/api/graph` so the reachability and node-info endpoints don't re-fetch the cluster on every click.

### Internals & cleanup

* New unit tests for `IsNodesReachable` covering allow, explicit-deny, default-deny, not-enforced, namespace-node-matcher, and multi-engine block paths.
* `test-data/` curated for showcasing — 13 debug fixtures dropped, the remaining set maps 1:1 to README scenarios (WAN⇆ wall, air-gapped, Istio DENY, L7 paths/methods/hosts, real `observability` namespace dump).
* `docs/arch/` pruned — stale ADRs that referenced abstractions never built (ambient-mode detection, L3/L4-only scope, edge-level intersection) deleted. `0002-backend-computed-intersection.md` revised in place to describe the as-built status-key intersection model.

---

## [0.1.0]
(2026-05-15)
6dae3463e21321b0baa360fabd195f978e235143
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
