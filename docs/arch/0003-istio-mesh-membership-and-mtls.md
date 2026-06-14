# ADR 0003: Istio Mesh Membership and mTLS Verdict

**Status:** Proposed
**Date:** 2026-06-01
**Branch:** `7-better-isio-support`

## Context

Current Istio support reads `AuthorizationPolicy` only — ALLOW + DENY rules, optional L7 hints captured per `policy.Rule`. Two gaps make the rendered graph misleading:

1. **Mesh membership unknown.** Engine evaluates AP regardless of whether selected workload is in the mesh. In ambient mode, a workload's namespace must carry `istio.io/dataplane-mode=ambient` for ztunnel to enforce policy. AP targeting a non-mesh workload is semantically dead but renders as effective.
2. **mTLS posture invisible.** `PeerAuthentication` (PA) determines whether a workload accepts cleartext, requires mTLS, or disables it. Users can't tell from the graph why a connection is rejected or allowed, nor which PA produced the current state.

User runs Istio in **ambient mode** — no sidecars, ztunnel + optional waypoint. Sidecar-mode signals (`istio-injection=enabled`, `sidecar.istio.io/inject`) are not in scope.

## Decision

Introduce a parallel **`MeshSource`** abstraction next to the existing `PolicySource`. A single Istio impl detects mesh membership from namespace labels and resolves PA hierarchy per workload. Results are stamped onto `WorkloadNode.Mesh`. Mesh detection runs **after** all `PolicySource.Evaluate` calls so engines stay mesh-agnostic.

### Data model

```go
// internal/mesh/mesh.go
type MeshMembership struct {
    InMesh   bool
    Provider string         // opaque: "istio"
    Mode     string         // dataplane mode: "ambient"
    Waypoint *WaypointRef   // nil = no L7 enforcement (ztunnel-only / L4)
    Mtls     *MtlsState     // nil if not-in-mesh
}

type WaypointRef struct {
    Namespace string   // waypoint Gateway namespace
    Name      string   // waypoint Gateway name
    Source    string   // "namespace-label" | "workload-label" | "service-account-label"
}

type MtlsState struct {
    Verdict       string             // workload-level resolved: "strict" | "permissive" | "disable"
    PortOverrides map[uint32]string  // port -> verdict, only when PA declared portLevelMtls; empty if none
    Sources       []MtlsSource       // every PA touching this workload, precedence order
}

type MtlsSource struct {
    Namespace string             // PA namespace
    Name      string             // PA name
    Scope     string             // "mesh" | "namespace" | "workload"
    Mode      string             // workload-level mode declared by this PA: "strict" | "permissive" | "disable" | "unset"
    PortModes map[uint32]string  // port-level modes declared by this PA, if any
    Winning   bool               // true for the entry that produced Verdict (workload-level)
}
```

`WorkloadNode` gains:

```go
Mesh *MeshMembership `json:"mesh,omitempty"`
```

Pointer keeps payload clean when annotation is absent. Generic field names (`Provider`, `Mode`) leave room for a future second provider without renaming.

### `MeshSource` interface

```go
// internal/mesh/mesh.go
type MeshSource interface {
    Name() string
    Evaluate(ctx context.Context, namespaces []string, indexByNS map[string]models.NSIndex) (EvaluationResult, error)
    ResolveMtls(ctx context.Context, workload models.WorkloadNode) (*MtlsState, error)
}

type EvaluationResult struct {
    Memberships map[NodeID]MeshMembership  // membership + waypoint binding resolved; Mtls field nil
    // future: Issues []Issue   // misconfig pass lands here
}
```

Two-method split:

- `Evaluate` runs during graph build. Fetches Gateways (cheap, needed to resolve waypoint binding for graph payload), reads ns labels, stamps `MeshMembership` w/ `InMesh`/`Provider`/`Mode`/`Waypoint`. Discards raw fetched resources after resolution — only the stamped binding survives.
- `ResolveMtls` runs at detail-endpoint and reachability-check time. Fetches PAs for workload's ns + root ns on demand, walks hierarchy, returns `MtlsState`. No app-level cache.

Why no PA cache:
- PA fetch per ns is tiny (handful of objects in practice)
- Most workloads in a graph never get inspected
- Always-fresh data — no staleness window between graph load + click
- One k8s round-trip per click is acceptable interactive latency

Why Gateways resolved during graph build (not on demand):
- Waypoint binding is exposed in graph payload for the badge / filter, so it has to be ready before the graph response is sent
- Single fetch per ns covers all workloads in that ns — amortized
- Raw Gateways aren't kept; only the resolved `WaypointRef` per workload sticks

### Mesh detection rules (Istio impl)

Per workload, in order:

1. Pod-template label `istio.io/dataplane-mode=none` → `InMesh: false` (opt-out). **Deferred to phase 2** — rare in ambient, single-line addition when needed.
2. Namespace label `istio.io/dataplane-mode=ambient` → `InMesh: true`, `Provider: "istio"`, `Mode: "ambient"`.
3. Otherwise → `InMesh: false`.

Cronjobs evaluated the same way — namespace label is authoritative when no live pod is available.

**CRD-absent degradation:** if Istio CRDs (`PeerAuthentication`, `Gateway`) aren't installed in cluster, fetch returns IsNotFound. Mesh source must catch + degrade: all workloads `InMesh: false`, empty `PeerAuths` / `Waypoints` maps. Graph build must not fail. Tested explicitly.

**Label-match parity:** PA `selector` matching, waypoint `use-waypoint` label resolution, and AP `selector` matching must use the same helper (`internal/utils/LabelsMatch`). Divergence between mesh + policy packages → silent verdict drift. Enforce by reuse, not by re-impl.

### PA hierarchy resolution (workload-level + port-level)

Resolver iterates PAs in precedence order (most-specific first), collecting every PA that touches the workload. Builds `Sources` slice. Two layers produced:

**Workload-level verdict** (`MtlsState.Verdict`):
- First non-`UNSET` `mode` in precedence order produces `Verdict`
- If all `UNSET` or no PA exists → `Verdict: "permissive"` (Istio default)

**Port-level overrides** (`MtlsState.PortOverrides`):
- For each port that any in-scope PA references via `portLevelMtls`, apply same precedence walk
- Workload-scope port overrides win over namespace-scope, win over mesh-scope
- Ports not referenced → not present in map; reachability code falls back to `Verdict`

Precedence order:

| Scope     | Match condition                                                                 |
|-----------|--------------------------------------------------------------------------------|
| workload  | PA in workload's namespace with `selector` matching workload labels             |
| namespace | PA in workload's namespace, no `selector`                                       |
| mesh      | PA in root namespace (`istio-system` or configured), no `selector`              |

`Winning` flag set on the first source whose `Mode != "unset"`. All others retained for the detail panel. Port-level modes declared by each PA stored on the same `MtlsSource` entry via `PortModes`.

### Waypoint binding resolution

Per workload, in order (most-specific first):

1. Pod-template label `istio.io/use-waypoint: <name>` or `none` (workload-scoped opt-in / opt-out)
2. ServiceAccount label `istio.io/use-waypoint: <name>` (per-SA)
3. Namespace label `istio.io/use-waypoint: <name>` (ns-scoped default)
4. None of the above → no waypoint (L4-only via ztunnel)

Special value `none` at any level → opt-out, no waypoint regardless of broader-scope binding.

Referenced waypoint resolved against cached `Gateway` resources w/ `gatewayClassName: istio-waypoint`. Cross-namespace waypoints supported via `<namespace>/<name>` label value.

Result stamped as `MeshMembership.Waypoint`. Nil when no binding or binding resolves to nonexistent Gateway (latter is a misconfig — flagged in future Issue pass).

### API surface — split payload

`MtlsState.Sources` is the heavy part of the struct and is only useful when a user is inspecting a specific workload. Graph response omits it. Detail endpoint returns it.

| Endpoint                              | `MeshMembership.InMesh / Provider / Mode` | `MeshMembership.Mtls`         |
|---------------------------------------|-------------------------------------------|-------------------------------|
| `GET /api/graph`                      | yes — needed for badges + filters         | **omitted** (`nil`)           |
| `GET /api/workload/:ns/:name/mesh`    | yes                                       | yes — full PA chain + verdict |

New endpoint shape (sketch):

```
GET /api/workload/{namespace}/{name}/mesh
→ MeshMembership { InMesh, Provider, Mode, Mtls { Verdict, Sources[] } }
```

Detail panel hits this on node click. Multi-node comparison view fans out one request per selected workload.

**Compute timing — no cache layer**

`mesh.ResolveMtls` fetches PAs on demand at the time of the detail / reachability request. No app-level cache. Fetch is small (per ns + root ns), happens only when a node is inspected or a reachability check is run.

Rules-style caching (`store.Cache.EvaluationResults`) intentionally not extended for PA / Gateway: rules are computed cluster-wide during graph build and partitioned per node for cheap detail reads — heavy compute, expensive recompute. mTLS resolution per workload is pure label-match + hierarchy walk, cheap, and avoiding the cache layer means no staleness window between graph load and click.

### Flow integration

```
Builder.BuildGraph(namespaces):
  1. fetch + buildNsIndex per namespace (parallel)        [unchanged]
  2. for each PolicySource: Evaluate                      [unchanged]
  3. for each MeshSource:   Evaluate                      [NEW]
        → fetches Gateways per ns, resolves waypoint binding
        → stamps WorkloadNode.Mesh (InMesh + Waypoint; Mtls left nil)
  4. updateStatusKeys (extended: emits in-mesh key from node.Mesh)
  5. renderEdges                                          [unchanged]

handleWorkloadMesh(ns, name):                             [NEW endpoint]
  → mesh.ResolveMtls(ctx, workload)                       // fetches PAs for workload ns + root ns
  → return MeshMembership{...,Mtls}

handleReachability(srcId, srcNs, dstId, dstNs):           [existing endpoint, extended]
  → mesh.ResolveMtls(ctx, dstWorkload)                    // one fetch per check
  → use dst's Waypoint (already stamped) for L7 enforceability
  → use dst's Mtls.PortOverrides[port] or .Verdict for accept/reject
```

No `Cache` extension. Existing `Cache` fields (`EvaluationResults`, `NsIndex`) unchanged.

### Reachability integration (primary consumer of `ResolveMtls`)

`store.IsNodesReachable(cache, srcId, srcNs, dstId, dstNs)` (`internal/store/store.go`) currently runs per-engine verdicts. Mesh data adds:

- **Both endpoints in mesh?** — read directly from each workload's stamped `MeshMembership.InMesh` (already in cache via `NsIndex` / stamped during graph build).
- **Will dst accept the call on the given port?** — call `mesh.ResolveMtls(ctx, dstWorkload)`. Use `MtlsState.PortOverrides[port]` if set, else `Verdict`. STRICT + non-mesh src → deny. DISABLE → cleartext only.
- **L7 rules enforceable on dst?** — `dst.Mesh.Waypoint != nil` (already stamped). AP w/ L7 fields targeting dst when no waypoint → enforcement degrades to L4 on that hop; reachability check must reflect this when explaining the verdict.

Each reachability call triggers one PA fetch (dst ns + root ns). Acceptable interactive latency.

mTLS status keys (`mtls-strict` etc.) emit during detail-endpoint serialization, not during graph build — they piggyback on `Mtls.Verdict`. Frontend filter on mTLS verdict requires either eager compute or a separate cluster-wide PA summary endpoint; defer until the filter is built.

Two registries, same loop pattern:

```go
graph/buildGraph.go:
  PolicySources(client) []policy.PolicySource    // existing
  MeshSources(client)   []mesh.MeshSource        // new — returns single Istio impl
```

### Status keys (added to `policy.AllStatusKeys()`)

- `in-mesh`, `not-in-mesh` — emitted during graph build from membership
- `mtls-strict`, `mtls-permissive`, `mtls-disabled` — emitted from detail endpoint only (lazy compute path). Vocabulary reserved in the catalog so future cluster-wide-filter work has a stable target.

`mtls-unset` never surfaces — resolver returns `permissive` default. Frontend filter sees stable vocabulary.

### Package layout

```
internal/mesh/
  mesh.go            # MeshMembership, MtlsState, MtlsSource, MeshSource interface, EvaluationResult
  istio/
    detect.go        # Istio MeshSource impl: orchestrates ns-label + PA reads
    peerauth.go      # PA hierarchy resolver (pure fn, takes raw PAs + workload labels)
    peerauth_test.go
    detect_test.go
```

Mirrors `internal/policy/` layout. Top-level `internal/mesh/` signals mesh detection is sibling to policy evaluation, not subordinate.

### KubernetesClient extension

```go
GetPeerAuthentications(ctx context.Context, namespace string) ([]*v1beta1.PeerAuthentication, error)
```

- Real `Client` impl: `istio.io/client-go` security v1 typed client
- `DemoClient`: embedded YAML fixtures, same pattern as `AuthorizationPolicy` fixtures already use

## Alternatives considered

**A. Overload `PolicySource.Evaluate` to also return mesh data.**

Add `Mesh map[NodeID]MeshMembership` to `EvaluationResult`. K8s engine returns empty map; Istio engine fills it. Rejected:
- Mixes concerns. Interface promises "produces rules"; mesh isn't a rule.
- Every future `PolicySource` must reason about a field it has nothing to say about.
- Mesh detection has different inputs (ns labels, PAs) than rule generation. Coupling them under one `Evaluate` makes both harder to test.

**B. Plain function `mesh.Annotate(...)`, no interface.**

Smallest code change. Rejected because once user explicitly asked for "same way as rules," the symmetric interface is the answer. Cost of one extra interface is one file; benefit is uniform mental model.

**C. Hard-wire mesh detection in `Builder.BuildGraph`.**

Fastest. Rejected — couples graph layer to Istio specifics. The whole point of the existing engine abstraction is to keep graph orchestration provider-neutral.

**D. Per-pod mesh-membership read (live `istio-proxy` container).**

Accurate ground truth for sidecar mode. Not applicable in ambient (no sidecar to detect). Skipped.

**E. Multi-mesh active simultaneously (`[]MeshMembership` per workload, precedence registry, conflict resolution).**

Speculative — only Istio is in scope. Single `*MeshMembership` field, single detector. Generic struct shape leaves the door open if a second provider lands later, but no scaffolding built for a scenario that doesn't exist.

## Consequences

**Positive:**
- Engines stay single-concern. Policy evaluation doesn't know about mesh; mesh detection doesn't know about rules.
- Mesh state visible to all downstream consumers (status keys, edge render, future misconfig pass) via one stamped field.
- Detail panel can show full PA chain + verdict — answers "why is this workload STRICT?" without users grepping the cluster.
- `MeshSource` interface symmetry means anyone who understands `PolicySource` already understands the new abstraction.
- Future misconfig pass (AP targets non-mesh workload, AP L7 without waypoint, PA STRICT on opt-out workload, etc.) fits naturally as `EvaluationResult.Issues` on the existing pass.

**Negative:**
- Two registries instead of one. Marginally more wiring in `Builder.BuildGraph`.
- Single `MeshSource` impl today — interface is documentation more than polymorphism. Accepted: the symmetry is the value.
- Two API surfaces (graph + per-workload detail) instead of one. Frontend gains a fetch on node click. Accepted: payload stays small, compute defers to actual interest.
- mTLS-related status keys don't apply at graph-build time (lazy path). Cluster-wide mTLS filter requires either switching to eager compute or adding a separate summary endpoint — flagged for whenever that filter ships.
- Each detail click + each reachability check incurs one k8s round-trip for PA fetch. Bounded (per ns + root ns), small payload, but visible if user clicks rapidly across many nodes. Cache layer can be added later if profiling shows hot path.

**Migration risk:**
- New `GetPeerAuthentications` method on `KubernetesClient`. `DemoClient` must implement to keep `DEMO_MODE=true` working. Mitigated: pattern matches existing `GetAuthorizationPolicies` exactly.
- `policy.AllStatusKeys()` vocabulary grows. Frontend filter UI absorbs new keys with no logic change — keys flow through the existing catalog.
- `updateStatusKeys` extension is the only edit to existing render code. Bounded.

## Out of scope (future work)

- Per-pod `dataplane-mode=none` opt-out
- Misconfiguration detector pass — needs own ADR. Inputs ready (mesh + PA + waypoint cached) so detectors are pure functions over cache contents
- PA-revision-aware resolution (multi-control-plane via `istio.io/rev=<rev>`) — single root namespace assumed
- Second mesh provider (Linkerd, Cilium mesh)
- Edge-level encryption state (`EdgeSecurity` struct) — dropped, no use case in ambient where in-mesh edges are always encrypted
- Cache TTL / eviction at large cluster scale (#10 / #11 from design review) — accepted risk
