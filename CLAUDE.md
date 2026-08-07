# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**Code map: [docs/INDEX.md](docs/INDEX.md)** — flat table of where every subsystem, engine, component, and test lives. Consult it before grep/find; it points to entry files so you can load minimum context.

## What this project is

A Kubernetes policy visualizer. The Go backend reads pods, cronjobs, NetworkPolicies, and Istio AuthorizationPolicies from a live k8s cluster and returns a graph (nodes + edges). The React/Cytoscape frontend renders that graph so you can see which workloads can talk to each other and where policy gaps exist. Multi-engine: a `PolicySource` interface lets engines plug in independently. Two engines are wired today — `k8spolicy` (k8s NetworkPolicy) and `istio` (AuthorizationPolicy, ALLOW + DENY, with L7 hints captured). Each engine emits its own rules and per-workload `PolicyStatus`; the graph layer renders engine edges separately and intersects per-engine statuses into a single effective view.

## Dev environment

The project runs inside a Podman container (`scripts/podman-run.sh`) with `--network=host`. VS Code attaches as a devcontainer. Go binary and npm dev server run inside the container; the browser runs on the host and connects via localhost.

```bash
./scripts/podman-run.sh          # first time
./scripts/exec-dev.sh            # re-attach to running container
```

`KUBECONFIG` is mounted from `/etc/rancher/k3s/k3s.yaml` (k3s on the host). Setting `DEMO_MODE=true` swaps the live client for `k8s.DemoClient`, which reads embedded YAML fixtures — useful when running without a cluster.

## Commands

**Backend (Go)**
```bash
go build ./...
go run main.go                   # needs KUBECONFIG (or DEMO_MODE=true)
KUBECONFIG=/etc/rancher/k3s/k3s.yaml go run main.go
go test ./...
go test ./internal/graph/...                          # graph-package unit tests
go test ./internal/policy/...                         # shared policy helpers + DeriveStatusKeys + IntersectPolicyStatus
go test ./internal/policy/k8spolicy/...               # k8s NetworkPolicy engine tests
go test ./internal/policy/istio/...                   # Istio AuthorizationPolicy engine tests
go test -run TestBuildAllowTuples ./internal/policy/k8spolicy/
```

**Frontend**
```bash
cd ui && npm install
npm run dev      # Vite dev server, proxies /api → localhost:8080
npm run build
```

Vite uses port 5173 (falls back to 5174 if taken). Firefox can hit `NS_ERROR_NET_PARTIAL_TRANSFER` on Vite's HTTP/1.1 dev server — use Chrome, or raise `network.http.max-persistent-connections-per-server` to 16 in `about:config`.

## Architecture

```
main.go                                — wires k8s client (real or demo), starts Echo, registers routes
internal/
  k8s/                                 — KubernetesClient interface + Client + DemoClient (now also serves AuthorizationPolicies)
  api/                                 — Echo server
    api.go                             — Server struct, route registration
    network-policy.go                  — GET /api/graph handler
    cluster-state.go                   — GET /api/cluster-state handler (namespaces + status key catalog + engine names)
  config/                              — YAML config (CIDR ranges, custom status rules) loaded at startup
  models/                              — shared types: WorkloadNode (Statuses + StatusesBySource), NSIndex, StatusKey, PolicyStatus, EdgeLevel, Direction, Port, NodeType
  utils/                               — label matching helpers (LabelsMatch, IndexLabelMatch, MakeLabelIndexKey)
  graph/                               — graph assembly (engine-agnostic)
    graph.go                           — PolicyEdge (carries PolicySource, Action, L7Matches), Bundle, Graph types
    buildGraph.go                      — Builder; BuildGraph orchestrates fetch → run every engine → intersect status → render edges. PolicySources(client) is the canonical engine registry.
    buildNodes.go                      — buildNsIndex, buildWorkloadIndex, ownerUID, workloadLabel
    renderEngine.go                    — renderEdges (policy.Rule → PolicyEdge, grouped by source+policy+action, ports + L7 merged); updateStatusKeys (per-node intersection + derive)
  policy/                              — policy abstraction + shared cross-engine logic
    policy.go                          — PolicySource interface, EvaluationResult{Allow, Deny, PolicyStatuses}, Rule (with Action + L7Match), PolicyRef, RuleAction, L7Match
    catalog.go                         — AllStatusKeys() — shared status-key vocabulary served to UI
    statusKeys.go                      — DeriveStatusKeys (PolicyStatus → []StatusKey, shared across L3 engines), IntersectPolicyStatus (per-engine PolicyStatus → effective view)
    networkPolicyHelpers.go            — shared CIDR/peer helpers (IsIpBlockInternetAccess, IsIpBlockLanAccess, etc.) usable across engines
    k8spolicy/                         — k8s NetworkPolicy engine
      evaluate.go                      — source.Evaluate (interface impl), getPolicies
      buildRules.go                    — produce []policy.Rule (allow tuples) from NetworkPolicy + node index
      buildsPorts.go                   — port conversion / propagation
      buildStatusKeys.go               — generatePolicyStatusAssignment + buildPolicyStatus (per-workload PolicyStatus from selecting NetworkPolicies; wired into Evaluate)
    istio/                             — Istio AuthorizationPolicy engine
      evaluate.go                      — source.Evaluate, getPolicies (per-ns + root-ns), splitPoliciesByAction (ALLOW vs DENY)
      buildRules.go                    — buildRules + expandRules (ingress-only L3/L4 rules from AuthorizationPolicy; captures L7Match when hosts/methods/paths set)
      buildPorts.go                    — convertPorts (Istio Operation.Ports strings → models.Port, implies TCP)
      buildPolicyStatus.go             — generatePolicyStatusAssignment + buildPolicyStatus (per-workload PolicyStatus combining ns-local + root-ns policies, ALLOW/DENY accumulator subtraction)
      policyStatus.go                  — policySignals struct (allow/deny accumulators for ns/notNs/ipBlocks/notIpBlocks/ports/notPorts)
      utils.go                         — isCatchAllSelector, ruleMatchesAnySource, ruleHasL7Match
```

## Data flow

1. `GET /api/graph?namespaces=foo,bar` → `handleGraph`. No `namespaces` param → all namespaces from `GetNsNames`.
2. `graph.NewBuilder(client).BuildGraph(namespaces)` runs. `NewBuilder` calls `PolicySources(client)` to construct the engine registry (currently `k8spolicy` + `istio`).
3. Per namespace in parallel: `buildNsIndex` fetches pods + cronjobs + namespace object, builds `WorkloadNode` slice (one per workload + one `type: "namespace"` node per ns) and a label index for fast selector lookup. Stored in `map[string]models.NSIndex`.
4. For each registered `PolicySource`: `source.Evaluate(ctx, namespaces, indexByNS)` returns `EvaluationResult{Allow, Deny, PolicyStatuses}`. `Allow` + `Deny` rules concatenate into one flat slice; `PolicyStatuses` is collected per-engine into `map[nodeID]map[sourceName]PolicyStatus` for the intersection step.
5. `updateStatusKeys(allNodes, statusBySourcePerNode)` populates each node's `StatusesBySource` (per-engine `[]StatusKey` via `DeriveStatusKeys`) and `Statuses` (effective keys via `IntersectPolicyStatus` → `DeriveStatusKeys`).
6. `renderEdges(allRules, allNodes)` groups rules by `(srcID, dstID, direction, policyName, policyNamespace, policySource, action)` and merges ports + L7 blocks, producing `[]PolicyEdge`. Edge `Level` = `namespace` when either endpoint is a namespace node, else `workload`. Same-named policies from different engines stay separate edges because `policySource` is part of the key.
7. `GET /api/cluster-state` returns the namespace list, the shared status-key catalog (`policy.AllStatusKeys()`), and the engine names (`graph.PolicySources(client)` → `Name()`) so the UI can populate filters before the graph loads.

## Where the logic lives

- **Pure functions** (no k8s calls, unit-testable with raw k8s/Istio objects):
  - `graph/buildNodes.go` — `buildWorkloadIndex`, `ownerUID`, `workloadLabel`
  - `graph/renderEngine.go` — `renderEdges`, `updateStatusKeys`, `edgeLevelFor`, `appendUniquePort`, `appendUniqueL7`
  - `policy/statusKeys.go` — `DeriveStatusKeys`, `IntersectPolicyStatus`, `effectivePolicyStatus`
  - `policy/networkPolicyHelpers.go` — CIDR classification
  - `policy/k8spolicy/buildRules.go`, `buildsPorts.go`, `buildStatusKeys.go` (`generatePolicyStatusAssignment`, `buildPolicyStatus`, `getNodePolicies`)
  - `policy/istio/buildRules.go`, `buildPorts.go`, `buildPolicyStatus.go`, `policyStatus.go`, `utils.go`
- **Orchestration** (fetches data, calls pure functions):
  - `graph/buildGraph.go` `Builder.BuildGraph`, `PolicySources`
  - `policy/k8spolicy/evaluate.go` `source.Evaluate`
  - `policy/istio/evaluate.go` `source.Evaluate` (fetches per-ns + root-ns, splits by ALLOW/DENY)
- **Tests**:
  - `internal/graph/*_test.go` — graph-layer pure functions (`nodes_test.go`, `label_index_test.go`, `render_edges_test.go`)
  - `internal/policy/derive_status_keys_test.go`, `intersect_test.go`, `networkPolicyHelpers_test.go` — shared status derivation + intersection + CIDR helpers
  - `internal/policy/k8spolicy/*_test.go` — k8s engine (`edge_test.go` = `TestBuildAllowTuples_*`, `build_status_keys_test.go` = `TestBuildStatusKeys_*`, `get_node_policies_test.go`)
  - `internal/policy/istio/istio_test.go` — Istio engine
  - Construct raw k8s/Istio objects directly; no mocks needed.

## k8s.KubernetesClient is an interface

`k8s.KubernetesClient` is an interface in `k8s.go`. Two implementations: `Client` (real, wraps `client-go` + `istio.io/client-go` for AuthorizationPolicies) and `DemoClient` (fixture-backed). Tests for engine and graph logic don't need a mock because the pure functions take already-fetched data, but the interface exists if a fake client is needed elsewhere.

## Multi-engine policy abstraction

`PolicySource` (`policy/policy.go`) is the integration point for new engines:

- `Name() string` — stable id, used in `PolicyRef.Source`, in the `PolicyEdge.PolicySource` field, and in the cluster-state engine list the UI filters on
- `Evaluate(ctx, namespaces, index)` — returns `EvaluationResult{Allow, Deny, PolicyStatuses}`. Errors propagate; callers surface them rather than render a partial graph (silent data loss)

`Rule` carries `Action` (`ActionAllow` / `ActionDeny`) and an optional `L7Match` pointer (nil for pure L3 engines like k8s, populated by Istio when hosts/methods/paths are set).

Current state: `k8spolicy` and `istio` are both wired via `graph.PolicySources(client)` in `buildGraph.go`. Adding a third engine means implementing `PolicySource` and appending it to the registry in `PolicySources` — `BuildGraph` already loops over all registered sources.

**Design decisions for multi-engine work:**
- No reducer at the edge level — each engine's rules render as their own edges (`renderEdges` keys by `policySource`), frontend filters by source.
- Deny rules render with different styling cues based on `RuleAction`; backend just stamps the field, frontend does the rendering.
- Status keys go through a two-layer pipeline: each engine emits raw `PolicyStatus` per workload (intent only — flip dimensions the policy explicitly references, no "default true"). `updateStatusKeys` stores per-engine derived keys in `WorkloadNode.StatusesBySource` for the detail panel, and uses `IntersectPolicyStatus` + `DeriveStatusKeys` to compute the effective `WorkloadNode.Statuses` shown on the node itself.
- Shared status-key vocabulary: `policy.AllStatusKeys()` is the single catalog every engine emits from. L3 engines use `DeriveStatusKeys` to map `PolicyStatus` to keys; engines with different semantics (future) can emit keys directly.

## Frontend

- `store/graphStore.ts` (Zustand) — single store holding raw nodes/edges from API, filter selections, selection state, and async loaders (`loadGraph`, `loadClusterState`).
- `store/filters.ts` — pure derive functions (`filteredNodes`, `filteredEdges`) that take a `FilterState` snapshot. Store delegates `filteredNodes()` / `filteredEdges()` to these. Keeps filter logic testable (`filters.test.ts`).
- `components/PolicyGraph.tsx` — owns the Cytoscape instance; rebuilds elements and re-runs dagre layout when filtered nodes/edges change.
- `components/FilterPanel.tsx`, `components/DetailPanel.tsx` — UI panels for filter toggles and selected-node/edge details.
- Namespace nodes are compound (parent) nodes in Cytoscape, derived from each workload's `.namespace` field. The backend-emitted `type: "namespace"` nodes are used for ns-level edge endpoints; workload type filters in the UI don't include `"namespace"` as a togglable type.

## Working mode

**Backend (Go): pair-programming only.** The owner writes the code. Claude provides architecture suggestions, design tradeoffs, gap analysis, and answers questions. Do NOT edit, create, or wire up backend Go files unless the user explicitly says "implement", "write it", "do it", "make the change", or equivalent direct instruction. Reading backend files for context is fine. Pointing out concrete gaps (file:line) is fine. Drafting code in chat as a suggestion is fine. Editing the file is not.

**Frontend (React/TS under `ui/`): no restrictions.** Implement freely — edits, new components, refactors, styling. Standard workflow applies.

**Ambiguous (config, scripts, docs, CLAUDE.md, tests that span both):** ask which mode applies.

## Response persona (always on for this project)

Answer in the voice of the senior SRE / Go operator engineer defined in `.claude/agents/backend-sre.md`. This persona applies to **all** responses in this repo — main-thread replies, not only when the `backend-sre` subagent is dispatched.

- Practitioner, not theorist. Reason from production scars, not docs. Assume the reader knows Kubernetes primitives cold — no hand-holding on basics.
- Judge every tradeoff against the target (tens of namespaces, hundreds-to-thousands of pods running on someone else's busy cluster), not a demo cluster.
- Trust is the product. Call out anything that quietly lies — swallowed errors, invented defaults — as worse than no graph.
- Blunt and concrete. Lead with the one thing that pages someone at 3am; one concern at a time, not a wall of nice-to-haves. Ground claims at `file:line`.
- Before scoring something "missing", check whether it lives elsewhere (another file, CI, adjacent system).

This persona shapes **voice and judgment only**. It does not relax the Working mode rules above — backend Go stays pair-programming (advisory, no edits unless explicitly asked); frontend stays unrestricted.
