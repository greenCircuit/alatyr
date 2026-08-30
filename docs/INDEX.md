# Code Index

Folder-level map. One line per directory: what lives there + why you'd open it.
Grep from the directory to find specific symbols. File-level detail lives in the code.

## Backend — top level

| Folder | Purpose |
|--------|---------|
| `main.go` (file) | Wires k8s client (real vs demo), loads config, starts Echo, registers routes |
| `embed.go` (file) | Embeds built UI + manifests into the Go binary |
| `e2e_test.go` (file) | End-to-end suite: CLI report mode + server boot against `test-data/scenarios/`, asserts exit codes and issue counts |
| `internal/` | All backend code (nothing importable from outside module) |
| `cli/` | Headless report mode (`-report`): scans a manifest dir, renders table to stdout or JSON to stdout/file, exits without starting the server |
| `scripts/` | Podman dev runner, container re-attach, image build, embed build |
| `chart/` | Helm chart for cluster deploy |
| `test-data/demo/` | Full curated demo cluster, embedded, served on `DEMO_MODE=true` |
| `test-data/scenarios/` | Small single-purpose fixture clusters for e2e, loaded with `-f <dir>` |
| `tests/` | Integration tests |
| `docs/` | Architecture notes, specs, backlog, reviews, this index |

## Backend — `internal/`

| Folder | Purpose |
|--------|---------|
| `internal/api/` | Echo HTTP handlers. One file per route family (graph, cluster-state, manifest, mesh-status, node-data, metrics) |
| `internal/graph/` | Assembles final graph: workload node index, edge rendering, per-node status intersection. Engine-agnostic — loops over registered `PolicySource`s |
| `internal/policy/` | Policy abstraction: `PolicySource` interface, `Rule`/`Action`/`L7Match` types, shared status catalog + derive/intersect, CIDR helpers, one subfolder per engine |
| `internal/policy/k8spolicy/` | k8s NetworkPolicy engine. Emits allow tuples + per-workload `PolicyStatus` |
| `internal/policy/istio/` | Istio AuthorizationPolicy engine. ALLOW + DENY, L7 hints, ns-local + root-ns policies, allow/deny signal accumulator |
| `internal/policy/calico/` | Calico GlobalNetworkPolicy/NetworkPolicy engine. Decodes raw CRDs, emits rules + status |
| `internal/mesh/` | Mesh detection dispatcher (interface + evaluate) |
| `internal/mesh/istio/` | Istio-specific mesh membership, PeerAuthentication, mTLS reachability, component detection |
| `internal/store/` | Cache/store layer: assembles what UI reads. Builds NodeInfo, neighbor lists, issues, reachability, layering. All API handlers read from here |
| `internal/k8s/` | KubernetesClient interface + two implementations: informer-backed live client and demo/fixture client |
| `internal/models/` | Shared types: Cache, WorkloadNode, PolicyStatus, StatusKey, Rule, Port, L7Match, MeshMembership, Issues, Reachability. No logic, just structs |
| `internal/config/` | YAML config load: CIDR ranges, custom status rules. Includes `defaultConfig.yaml` |
| `internal/utils/` | Cross-package helpers (label matching) |
| `internal/logging/` | slog setup + Echo middleware + request context |
| `internal/thirdparty/` | Vendored third-party code |

## Backend — key landmarks (for quick jumps)

| What | Where |
|------|-------|
| PolicySource interface | `internal/policy/policy.go:9` |
| Engine registry (add new engine) | `internal/graph/buildGraph.go` (`BuildGraph`) |
| Status derive + intersect | `internal/policy/statusKeys.go` |
| Shared status catalog | `internal/policy/catalog.go` |
| Route registration | `internal/api/api.go` |

## Frontend — top level (`ui/`)

| Folder | Purpose |
|--------|---------|
| `ui/src/` | React app source |
| `ui/public/` | Static assets served as-is |
| `ui/dist/` | Vite build output (gitignored, embedded by backend) |
| `ui/*.md` | Style guide, design principles, filter panel UX review, README |
| `ui/*.config.*`, `tsconfig.*`, `package.json` | Vite, ESLint, TS, npm config |

## Frontend — `ui/src/`

| Folder | Purpose |
|--------|---------|
| `ui/src/main.tsx`, `App.tsx`, `App.css`, `index.css` (files) | Vite entry + root component + global styles |
| `ui/src/api/` | HTTP client for backend routes (`/api/*`) |
| `ui/src/store/` | Zustand stores + pure derive functions. `graphStore` = raw graph + filters + selection. `manifestStore` = manifest fetch. `filters.ts`, `clusterStats.ts`, `policyTableEdges.ts`, `issueIndex.ts` are pure derives (testable, `.test.ts` alongside) |
| `ui/src/data/` | Engine metadata: names, icons (React components + module.css) |
| `ui/src/style/` | Design tokens: color CSS vars, utility classes, graph edge styles, graph node/edge tokens |
| `ui/src/assets/` | Static assets referenced from components |
| `ui/src/components/` | UI panels + views (see below) |

## Frontend — `ui/src/components/`

Most UI code lives here. Each panel is a folder with `index.tsx` as entry + a scoped `*.module.css`.
Subfolder convention: `parts/` = private components split only for size, `views/` = swappable subject-scoped screens, `shared/` = cross-view primitives + pure derives.

### `PolicyGraph/` — Cytoscape graph surface

| Path | Purpose |
|------|---------|
| `index.tsx` | Mounts Cytoscape instance, wires store → elements, runs dagre layout on filtered data change |
| `PolicyGraph.module.css` | Container + overlay styles |
| `parts/elements.ts` | Convert store nodes/edges → Cytoscape element list |
| `parts/styles.ts` | Cytoscape stylesheet (node/edge visual rules) |
| `parts/bundling.ts` | Group parallel edges between same nodes into visual bundles |
| `parts/coverage.ts` | Coverage overlay computation (policy-coverage highlight) |
| `parts/issueMarkers.ts` | Compute per-node/edge issue badge positions |
| `parts/legend.tsx` + `Legend.module.css` | Floating legend UI |
| `parts/*.test.ts` | Unit tests for bundling / coverage / issue markers |

### `FilterPanel/` — left rail filters

| Path | Purpose |
|------|---------|
| `index.tsx` | Panel shell; reads store filter state, dispatches setters |
| `FilterPanel.module.css` | Panel + dropdown styles |
| `parts/filter-dropdowns.tsx` | Filter dropdown components (namespaces, engines, status keys, types) |
| `parts/display-dropdown.tsx` | Display-mode toggle (graph / tables / cluster status) |
| `parts/constants.ts` | Static option lists / labels |
| `parts/use-filter-reset.ts` | Hook: reset filters on nav / cluster-state reload |
| `parts/useOutsideClick.ts` | Hook: close dropdown on outside click |

### `DetailPanel/` — right rail (selected node/edge)

| Path | Purpose |
|------|---------|
| `index.tsx` | Routes selection → correct view (workload / edge / bundle / reachability) |
| `DetailPanel.module.css` | Panel + section styles |
| `views/WorkloadView.tsx` | Selected workload: identity, status, mesh, issues, neighbors |
| `views/EdgeView.tsx` | Selected edge: policy source, action, ports, L7 hints |
| `views/PolicyBundleView.tsx` | Multiple policies covering same pair — grouped |
| `views/ReachabilityView.tsx` | Reachability answer for a selected pair |
| `shared/status.tsx` | Render `PolicyStatus` + `StatusKey` badges |
| `shared/issues.tsx` | Render `Issues` list with severity |
| `shared/mesh.tsx` | Render mesh membership + mTLS badges |
| `shared/edge-composites.tsx` | Compose edge summaries from raw edges |
| `shared/badges.tsx` | Small badge primitives (engine, action, level) |
| `shared/rows.tsx` | Label/value row primitives |
| `shared/presentation.ts` | Formatting helpers (labels, ports, actions) |
| `shared/StatusIcon.tsx` | Status-key icon component |
| `shared/MtlsChip.tsx` | mTLS status chip |
| `shared/ManifestModal.tsx` | Modal showing raw YAML of a selected resource |
| `shared/groupEdgesByPair.ts` | Pure derive: group edges by (src,dst) |
| `shared/groupNeighborsByPolicy.ts` | Pure derive: group neighbors by covering policy |
| `shared/reachability.ts` | Pure derive: reachability computation |
| `shared/*.test.ts` | Unit tests for pure derives |

### `TablesView/` — tabular alternative to graph

| Path | Purpose |
|------|---------|
| `index.tsx` | Tab shell + active-table routing |
| `CulpritActions.tsx` | Action row for culprit rows (drill-down / open detail) |
| `WorkloadsTable.tsx` | Workloads with status + issues per row |
| `PoliciesTable.tsx` | Policies with source, action, selected coverage |
| `IssuesTable.tsx` | Flat issue list |
| `StatusRollup.tsx` | Top-of-table status-key rollup counts |
| `IssueRollup.tsx` | Top-of-table issue rollup counts |
| `EngineRollup.tsx` | Top-of-table engine rollup counts |
| `Rollup.module.css` | Rollup styles |
| `IssuesPopover.tsx` + `IssuesPopover.module.css` | Row-hover popover listing issues |
| `SortHeader.tsx` + `SortHeader.test.ts` | Sortable column header + tests |

### `ClusterStatusView/` — cluster-wide dashboard

| Path | Purpose |
|------|---------|
| `index.tsx` | Dashboard shell + section layout |
| `Panel.module.css` | Shared panel container styles |
| `StatCards.tsx` | Top-row stat tiles (workloads, policies, issues, coverage %) |
| `ClusterMeshSummary.tsx` | Mesh overview (membership, mTLS mode) |
| `MeshRollup.tsx` | Rollup of mesh state |
| `NamespaceTable.tsx` | Per-namespace summary rows |
| `RiskyWorkloadsTable.tsx` | Workloads sorted by issue severity |
| `CoverageBar.tsx` | Policy coverage bar chart |
| `ProportionBar.tsx` | Reusable proportion bar |
| `ExposedCallout.tsx` + `exposed-chip.module.css` | Callout for internet-exposed workloads |
| `status-table.module.css` | Shared table styles |
| `calicoselector/` | Calico-specific selector UI (nested subfolder) |

### `IssuesDrawer/` — bottom drawer

| Path | Purpose |
|------|---------|
| `index.tsx` | Drawer shell + issue list |
| `IssuesDrawer.module.css` | Drawer styles |

### `DesignPreview/` — sandbox

| Path | Purpose |
|------|---------|
| `DesignPreview/` | Non-shipped design iteration surface. Reference for LIVE restyle work (see `project_live_panel_restyle` memory) |

## Frontend — reference docs

| File | Purpose |
|------|---------|
| `ui/STYLEGUIDE.md` | Concrete style tokens + patterns. Consult before adding UI |
| `ui/DESIGN_PRINCIPLES.md` | UX principles for the surface |
| `ui/FILTERPANEL_UX_REVIEW.md` | Filter panel critique + direction |
| `ui/README.md` | Dev setup, Vite quirks |

## Repo docs — `docs/`

| Folder / file | Purpose |
|---------------|---------|
| `docs/INDEX.md` | This file |
| `docs/FEATURES.md` | Feature notes |
| `docs/TODO.md` | Work backlog |
| `docs/devops-improvement-backlog.md` | Devops-track backlog |
| `docs/status-dashboard-spec.md` | Cluster status dashboard spec |
| `docs/status-key-computation.md` | How status keys are computed end-to-end |
| `docs/egress-cidr-reachability.md` | Egress + CIDR reachability model |
| `docs/informers.md` | Informer-based client notes |
| `docs/arch/` | Architecture notes per subsystem |
| `docs/backlog/` | Longer-form backlog items |
| `docs/issues/` | Issue writeups |
| `docs/stories/` | User stories |
| `docs/reviews/` | Audits, MR summaries, design reviews |

## Deploy / packaging

| File / folder | Purpose |
|---------------|---------|
| `Dockerfile` | Runtime image |
| `Dockerfile.sandbox` | Sandbox image variant |
| `Dockerfile.tests` | Test-runner image |
| `chart/` | Helm chart |
| `scripts/build-embedded.sh` | Build UI + embed into Go binary |
| `scripts/buildImg.sh` | Container image build |
| `scripts/podman-run.sh` | First-time podman container start |
| `scripts/exec-dev.sh` | Re-attach to running dev container |
