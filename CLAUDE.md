# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A Kubernetes NetworkPolicy visualizer. The Go backend reads pods, services, and NetworkPolicies from a live k8s cluster and returns a graph (nodes + edges). The React/Cytoscape frontend renders that graph so you can see which workloads can talk to each other and where policy gaps exist.

## Dev environment

The project runs inside a Podman container (`scripts/podman-run.sh`) with `--network=host`. VS Code attaches to that container as a devcontainer. The Go binary and npm dev server both run inside the container; the browser runs on the host and connects via localhost (host networking bridges them).

Start the container:
```bash
./scripts/podman-run.sh          # first time
./scripts/exec-dev.sh            # re-attach to running container
```

`KUBECONFIG` is mounted from `/etc/rancher/k3s/k3s.yaml` (k3s cluster on the host).

## Commands

**Backend (Go)**
```bash
go build ./...
go run main.go                   # needs KUBECONFIG env var
KUBECONFIG=/etc/rancher/k3s/k3s.yaml go run main.go
go test ./...
go test ./internal/graph/...     # unit tests only (no k8s needed)
go test -run TestBuildEdges ./internal/graph/
```

**Frontend**
```bash
cd ui && npm install
npm run dev      # Vite dev server, proxies /api → localhost:8080
npm run build
```

Vite runs on port 5173 (falls back to 5174 if 5173 is taken). Firefox has issues with `NS_ERROR_NET_PARTIAL_TRANSFER` on Vite's HTTP/1.1 dev server — use Chrome, or raise `network.http.max-persistent-connections-per-server` to 16 in `about:config`.

## Architecture

```
main.go
  └── internal/k8s    — k8s.Client struct, thin wrappers over client-go (GetPods, GetSvc, GetPolicies, GetNs, GetNsNames)
  └── internal/api    — Echo HTTP server, single route GET /api/graph
  └── internal/graph  — all graph logic
        graph.go      — shared types (WorkloadNode, PolicyEdge, Graph, enums)
        workload.go   — Builder struct; BuildGraph orchestrates fetching + building
        nodes.go      — pure functions: build workload nodes from pods/svcs, label matching
        edge.go       — pure functions: build policy edges from NetworkPolicy objects
```

**Key data flow:**
1. `GET /api/graph?namespaces=foo,bar` hits `handleGraph`
2. `graph.NewBuilder(client).BuildGraph(namespaces)` is called
3. Per namespace: fetch pods+svcs → build `WorkloadNode` slice → fetch namespace object → append namespace node
4. Fetch all `NetworkPolicy` objects → `buildEdges(nodesByNS, policies)` → `[]PolicyEdge`
5. Return `Graph{Nodes, Edges}` as JSON

**Where the logic lives:**
- `nodes.go` and `edge.go` contain pure functions (no k8s calls) — this is where correctness logic lives and where unit tests go
- `workload.go` (`Builder`) is thin orchestration — it only fetches data and calls the pure functions
- Tests in `edge_test.go` and `nodes_test.go` construct raw k8s objects and call the pure functions directly; no interface or mock needed

**Frontend:**
- `graphStore.ts` (Zustand) holds all state: raw nodes/edges from API, filter selections, derived filtered views
- `PolicyGraph.tsx` owns the Cytoscape instance; re-builds elements and re-runs dagre layout whenever filter state changes
- Namespace nodes are compound (parent) nodes in Cytoscape, derived from workload nodes' `.namespace` field — the `type: "namespace"` nodes from the backend are filtered out in `filteredNodes()` since `selectedNodeTypes` only tracks workload types

## k8s.Client has no interface

`k8s.Client` is a plain exported struct, not an interface. All k8s methods are getters. Tests for graph logic don't need a mock because `nodes.go`/`edge.go` are pure functions that take already-fetched data. If you need to test `Builder.BuildGraph` end-to-end, use a real or kind cluster.
