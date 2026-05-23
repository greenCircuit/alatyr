# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A Kubernetes NetworkPolicy visualizer. The Go backend reads pods, services, cronjobs, and NetworkPolicies from a live k8s cluster and returns a graph (nodes + edges). The React/Cytoscape frontend renders that graph so you can see which workloads can talk to each other and where policy gaps exist. A multi-engine policy migration is in progress — the architecture supports additional policy sources (Istio L3/L4 AuthorizationPolicy planned next) behind a `PolicySource` interface.

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
go test ./internal/policy/k8spolicy/...               # k8s policy engine tests
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
  k8s/                                 — KubernetesClient interface + Client + DemoClient
  api/                                 — Echo server
    api.go                             — Server struct, route registration
    network-policy.go                  — GET /api/graph handler
    cluster-state.go                   — GET /api/cluster-state handler (namespaces + status key catalog)
  config/                              — YAML config (CIDR ranges, custom status rules) loaded at startup
  models/                              — shared types: WorkloadNode, NSIndex, StatusKey, EdgeLevel, Direction, Port, NodeType
  utils/                               — label matching helpers (LabelsMatch, IndexLabelMatch, MakeLabelIndexKey)
  graph/                               — graph assembly (engine-agnostic)
    graph.go                           — PolicyEdge, Bundle, Graph types
    workload.go                        — Builder; BuildGraph orchestrates fetch → evaluate → fold
    build-nodes.go                     — buildNsIndex, buildWorkloadIndex, getNodePolicies (per-node policy match)
    fold-edges.go                      — foldTuplesToEdges (policy.Rule → PolicyEdge, grouped + ports merged)
  policy/                              — policy abstraction
    policy.go                          — PolicySource interface, EvaluationResult, Rule, PolicyRef, CoverageMode, StatusKeyAssignment
    networkPolicyHelpers.go            — shared CIDR/peer helpers (IsIpBlockInternetAccess, etc.) usable across engines
    k8spolicy/                         — k8s NetworkPolicy engine
      evaluate.go                      — source.Evaluate (interface impl), getPolicies, GetNodePolicies
      buildRules.go                    — produce []policy.Rule (allow tuples) from NetworkPolicy + node index
      buildsPorts.go                   — port conversion / propagation
      buildStatusKeys.go               — per-node status badge computation (NOT yet wired into Evaluate)
```

## Data flow

1. `GET /api/graph?namespaces=foo,bar` → `handleGraph`. No `namespaces` param → all namespaces from `GetNsNames`.
2. `graph.NewBuilder(client).BuildGraph(namespaces)` runs.
3. Per namespace in parallel: `buildNsIndex` fetches pods + cronjobs + namespace object, builds `WorkloadNode` slice (one per workload + one `type: "namespace"` node per ns) and a label index for fast selector lookup. Stored in `map[string]models.NSIndex`.
4. `k8spolicy.New(client).Evaluate(ctx, namespaces, indexByNS)` fetches all `NetworkPolicy` objects and returns an `EvaluationResult` with `Allow []policy.Rule`. Each `Rule` carries `Contributors []PolicyRef` identifying which policy/source produced it.
5. `foldTuplesToEdges(result.Allow, allNodes)` groups rules by `(srcID, dstID, direction, policy)` and merges ports, producing `[]PolicyEdge`. Edge `Level` = `namespace` when either endpoint is a namespace node, else `workload`.
6. `GET /api/cluster-state` returns the namespace list + status-key catalog (`k8spolicy.GetStatusKeys`) so the UI can populate filters before the graph loads.

## Where the logic lives

- **Pure functions** (no k8s calls, unit-testable with raw k8s objects):
  - `graph/build-nodes.go` — `buildWorkloadIndex`, `getNodePolicies`
  - `graph/fold-edges.go` — `foldTuplesToEdges`
  - `policy/k8spolicy/buildRules.go`, `buildsPorts.go`, `buildStatusKeys.go`
  - `policy/networkPolicyHelpers.go` — CIDR classification
- **Orchestration** (fetches data, calls pure functions):
  - `graph/workload.go` `Builder.BuildGraph`
  - `policy/k8spolicy/evaluate.go` `source.Evaluate`
- **Tests**: `internal/graph/*_test.go` for graph-layer pure functions; `internal/policy/k8spolicy/*_test.go` for engine logic (`edge_test.go` = `TestBuildAllowTuples_*`, `build_status_keys_test.go` = `TestBuildStatusKeys_*`). Construct raw k8s objects directly; no mocks needed.

## k8s.KubernetesClient is an interface

`k8s.KubernetesClient` is an interface in `k8s.go`. Two implementations: `Client` (real, wraps `client-go`) and `DemoClient` (fixture-backed). Tests for engine and graph logic don't need a mock because the pure functions take already-fetched data, but the interface exists if a fake client is needed elsewhere.

## Multi-engine policy abstraction

`PolicySource` (`policy/policy.go`) is the integration point for new engines:

- `Name() string` — stable id, used in `PolicyRef.Source` and the planned `?policySource=` filter
- `Coverage(node, dir)` — `DefaultAllow` / `DefaultDeny` / `NotApplicable` (e.g. Istio + out-of-mesh workload)
- `Evaluate(ctx, namespaces, index)` — returns `EvaluationResult{Allow, Deny, StatusKeys}`

Current state: only `k8spolicy` implemented; `workload.go:55` calls it directly. Adding an engine means implementing the interface, wiring it into a source loop in `BuildGraph`, and letting the frontend filter on `PolicyEdge` source attribution.

**Design decisions for multi-engine work:**
- No reducer / intersection logic — each engine's rules become their own edges, frontend filters by source.
- Deny rules (future, e.g. Istio AuthZ `action: DENY`) get different color/badge and a separate filter. Backend decides styling cues; frontend just renders.
- Status keys: each engine populates its own `StatusKeys` independently; badges are additive, no composition. K8s engine's `BuildStatusKeys` is not yet wired into `Evaluate` — known gap.

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
