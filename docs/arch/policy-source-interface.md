# Policy Source Interface

Spec for `internal/policy/`. Read alongside [multi-source-policy-graph.md](multi-source-policy-graph.md).

## Interface

```go
package policy

type PolicySource interface {
    // Stable identifier. Used as map key in Edge.Contributors and as
    // value in the API ?policySource= param.
    Name() string

    // Fetch all policy objects for the requested namespaces.
    // Implementations cache results internally; subsequent calls in the
    // same request are no-ops.
    Fetch(ctx context.Context, namespaces []string) error

    // Coverage reports whether this source is enforcing on the given
    // workload in the given direction.
    //   DefaultAllow  — no policy of this source selects the workload+dir
    //   DefaultDeny   — at least one policy selects it; only explicit
    //                   AllowTuples will produce edges
    //   NotApplicable — source cannot enforce here at all
    //                   (e.g. Istio + out-of-mesh workload)
    Coverage(node *WorkloadNode, dir Direction) CoverageMode

    // AllowTuples returns explicit allow rules expanded to
    // pod-level tuples. Only meaningful for workloads with
    // Coverage == DefaultDeny.
    AllowTuples(nodes []WorkloadNode) []AllowTuple

    // DenyTuples returns explicit deny rules (Istio DENY action, etc.).
    // Subtracted from the intersection result regardless of which
    // source allowed the tuple.
    DenyTuples(nodes []WorkloadNode) []AllowTuple

    // StatusKeys reports per-workload diagnostics owned by this source.
    // Aggregated by the graph layer onto WorkloadNode.Statuses.
    StatusKeys(nodes []WorkloadNode) []StatusKeyAssignment
}
```

## Types

```go
type AllowTuple struct {
    SrcID, DstID string       // workload-node IDs or CIDR strings for IP-block peers
    Port         Port         // existing graph.Port type
    Direction    Direction    // ingress | egress (egress only meaningful for sources that enforce it)
    Contributors []PolicyRef  // policies in THIS source that produced the tuple
}

type PolicyRef struct {
    Source    string // matches PolicySource.Name()
    Name      string // policy object name
    Namespace string
    RuleIndex int    // 0-based index into spec.ingress / spec.egress / spec.rules
}

type CoverageMode int

const (
    CoverageDefaultAllow CoverageMode = iota
    CoverageDefaultDeny
    CoverageNotApplicable
)

type StatusKeyAssignment struct {
    WorkloadID string
    Key        StatusKey
}
```

`WorkloadNode`, `Direction`, `Port`, `StatusKey` remain in `internal/graph/` (existing).

## Registry

```go
package policy

func Register(s PolicySource)                   // called from init() in each source package
func GetEnabled(names []string) []PolicySource  // filters registered set by API param
```

`internal/policy/k8s/source.go` and `internal/policy/istio/source.go` register themselves via `init()`. The API handler whitelists allowed names; unknown names return 400.

## Intersection algorithm

```
inputs:  sources []PolicySource, nodes []WorkloadNode
output:  []EffectiveTuple

1. For each source s in sources:
     coverage[s][workload][direction] = s.Coverage(workload, direction)
     allowSet[s] = set of AllowTuples (keyed by (SrcID, DstID, Port, Direction))
     denySet[s]  = set of DenyTuples  (same key)

2. Build candidate tuple universe U:
     U = union of allowSet[s].keys() across all s
     Plus: for each (src,dst,port,dir) where SOME source has coverage=DefaultDeny,
           include matching tuples from other sources too.

   (Workloads where every source has DefaultAllow or NotApplicable do not
    appear in U. They are unconstrained — render as no edge OR as a
    catch-all "no policy" view per UI mode. Existing K8s-only behavior
    drops them; preserved here.)

3. For each tuple t in U:
     allowedBySource[s] =
         coverage[s][t.dst][t.direction] in {DefaultAllow, NotApplicable}
         OR t in allowSet[s]
     deniedBySource[s] = t in denySet[s]

     t.effective = (allowedBySource[s] for all s) AND (NOT deniedBySource[s] for any s)

4. Keep tuples where t.effective = true.
   Merge contributors across sources:
     t.Contributors[s] = allowSet[s][t].Contributors  (when t was explicit there)
                       = []                            (when source was DefaultAllow / NotApplicable)
```

### Tuple-to-edge collapse

Tuples come out at pod granularity. Frontend wants the most specific edge that still covers every surviving tuple for a (srcGroup, dstGroup) pair:

```
group surviving tuples by (srcWorkloadID, dstWorkloadID, direction)
  → workload-level edge with merged port list

OR (when grouping by workload would create an edge per pod in NS):

group by (srcNS, dstNS) when every pod in srcNS+dstNS is present
  → NS-level edge (matches existing collapse behavior in edge.go)
```

The collapse rule is identical to today's K8s NP behavior — `EdgeLevelNamespace` when peer matches the whole NS, `EdgeLevelWorkload` otherwise. Contributors per source are preserved through the collapse.

## Adding a new source (Calico example)

1. Create `internal/policy/calico/source.go`.
2. Implement the five `PolicySource` methods.
3. `init() { policy.Register(&calicoSource{}) }`.
4. Add `"calico"` to the API param whitelist in `internal/api/`.
5. Add status-key constants under `calico.*` namespace if needed.

No edits to `intersect.go`, no edits to existing sources.

## Testing approach

- `internal/policy/k8s/` keeps the existing `edge_test.go` style: construct raw k8s objects, call the source's `AllowTuples` directly, assert tuples.
- `internal/policy/istio/` mirrors it with Istio AuthPolicy fixtures.
- `internal/policy/intersect_test.go` uses a `fakeSource` test double (in-memory tuple sets + coverage map) to exercise the engine without touching K8s or Istio.
- End-to-end against a real cluster only required for `internal/graph/build.go` orchestration.
