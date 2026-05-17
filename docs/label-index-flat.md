# Flat label index — proposed change

Replaces linear label-scan calls in `findNodeByLabel` with a single-level `map[string][]*WorkloadNode` indexed by `"key=value"`.

Supersedes the nested-map proposal in `graph-efficiency.md` §2 — flat is simpler with equivalent O(1) lookup.

---

## Current implementation

`findNodeByLabel` in `internal/graph/build-nodes.go:170`:

```go
func findNodeByLabel(labelMap map[string]string, nodes []WorkloadNode) []WorkloadNode {
    var matches []WorkloadNode
    for _, node := range nodes {           // N nodes
        matched := true
        for k, v := range labelMap {       // L selector labels
            if node.Labels[k] != v {
                matched = false
                break
            }
        }
        if matched {
            matches = append(matches, node)
        }
    }
    return matches
}
```

**Cost per call:** `O(N · L)` worst case.

**Call sites:**
- `build-nodes.go:70` — `buildWorkloadNodesForNS` matching services to pod workloads
- `build-nodes.go:126` — `getSourceNodes` matching policy podSelector to nodes in ns
- `build-edges.go:84,117` — `generateEdges` matching peer podSelector to nodes per ns

**Per request cost** with `NS` namespaces × `P` policies × `R` peers per policy × `N` nodes per ns × `L` labels per selector:

```
O(NS² · P · R · N · L)
```

---

## Proposed implementation

### Data structure

```go
// One LabelIndex per namespace. Key format: "labelKey=labelValue"
type LabelIndex map[string][]*WorkloadNode

// Stored alongside nodesByNS during build
type NSIndex struct {
    workloads  map[string][]WorkloadNode
    nsNodes    map[string]*WorkloadNode   // ns → namespace node (O(1))
    labels     map[string]LabelIndex      // ns → label index
}
```

### Helper

```go
func labelKey(k, v string) string {
    return k + "=" + v
}
```

### Build (per namespace, once)

```go
// buildWorkloadIndex builds a flat label index for fast selector matching.
// Pointers reference the caller's slice — do not append to nodes after this returns.
func buildWorkloadIndex(nodes []WorkloadNode) map[string][]*WorkloadNode {
    index := make(map[string][]*WorkloadNode, len(nodes))
    for position := range nodes {
        node := &nodes[position]
        for key, value := range node.Labels {
            entry := makeLabelIndexKey(key, value)
            index[entry] = append(index[entry], node)
        }
    }
    return index
}
```

**Build cost:** `O(N · L)` per namespace.

### Lookup (replaces `findNodeByLabel`)

Use an explicit `first` boolean to mark the seed iteration. Trades one variable for readability over the `nil`-vs-empty-map distinction.

```go
// indexLabelMatch returns nodes that match every label in the selector.
// Empty selector returns nil — catch-all handled by isCatchAll before calling.
func indexLabelMatch(selector map[string]string, labelIndex map[string][]*WorkloadNode) []*WorkloadNode {
    if len(selector) == 0 {
        return nil
    }

    result := map[*WorkloadNode]bool{}
    first := true

    for key, value := range selector {
        bucket, exists := labelIndex[makeLabelIndexKey(key, value)]
        if !exists {
            return nil
        }

        if first {
            // seed result from the first bucket
            for _, node := range bucket {
                result[node] = true
            }
            first = false
            continue
        }

        // intersect: keep only nodes that appear in both result and bucket
        next := map[*WorkloadNode]bool{}
        for _, node := range bucket {
            if result[node] {
                next[node] = true
            }
        }
        result = next
        if len(result) == 0 {
            return nil
        }
    }
    output := make([]*WorkloadNode, 0, len(result))
    for node := range result {
        output = append(output, node)
    }
    return output
}
```

### Walkthrough

Three workloads:

```
web-app:  {app: web, tier: frontend}
api:      {app: api, tier: backend}
worker:   {app: api, tier: backend}
```

Resulting index buckets:

```
"app=web"        → [web-app]
"app=api"        → [api, worker]
"tier=frontend"  → [web-app]
"tier=backend"   → [api, worker]
```

Selector `{app: api, tier: backend}`:

| Step | Pair | Bucket | first | Result set |
|---|---|---|---|---|
| 1 | `app=api` | `[api, worker]` | true → seed | `{api, worker}` |
| 2 | `tier=backend` | `[api, worker]` | false → intersect | `{api, worker}` |
| 3 | done | | | returns `[api, worker]` |

Selector `{app: web, tier: backend}` (impossible combo):

| Step | Pair | Bucket | first | Result set |
|---|---|---|---|---|
| 1 | `app=web` | `[web-app]` | true → seed | `{web-app}` |
| 2 | `tier=backend` | `[api, worker]` | false → intersect | `{}` empty |
| 3 | early exit | | | returns nil |

Intersection narrows — result can only shrink with each label, never grow. Early exit on empty saves remaining work.

**Lookup cost:** `O(L · k)` where k = avg matches per selector pair (typically 1-5).

---

## Comparison

| Metric | Current | Proposed (flat) |
|---|---|---|
| Lookup time | `O(N · L)` | `O(L · k)` |
| Build time | none | `O(N · L)` per ns, once |
| Edge-building total | `O(NS² · P · R · N · L)` | `O(NS² · P · R · L · k)` |
| Speedup factor | — | `N / k` |
| Memory | nodes only | nodes + label index (~N · L pointers per ns) |

**Example cluster:** 10 ns × 30 nodes × 20 policies × 3 peers × 4 labels.

- Current: ~720,000 label comparisons per request
- Proposed: ~48,000 ops (with k≈2)
- **~15× reduction**

Speedup scales with `N/k`. Small clusters: modest gain. Large clusters with hundreds of workloads per ns: order-of-magnitude.

---

## Comparison: flat vs nested (alternative)

Existing `graph-efficiency.md` proposes nested `map[k]map[v][]*WorkloadNode`. Both give O(1) lookup. Differences:

| Op | Flat | Nested |
|---|---|---|
| Build per label | 1 string concat + 1 probe | 0 concat + 1-2 probes |
| Lookup per pair | 1 string concat + 1 probe | 0 concat + 2 probes |
| Code complexity | low (one map type) | medium (nested nil checks) |
| Memory | smaller | +200-500B per ns (extra inner map headers) |

Picked flat: code clarity beats microsecond differences. Selector matching runs ~hundreds of times per request — string concat cost is microseconds total.

---

## Return type: pointers vs indices vs copies

Three options for what `Match` returns.

### Option 1: Pointers (`[]*WorkloadNode`) ✓ chosen

Pointer identity makes intersection O(1) — the pointer address is the map key, no field comparison needed:

```go
seen := map[*WorkloadNode]bool{}
seen[nodeA] = true
seen[nodeA] == true  // same address → instant hit
```

Risk: if the source slice is appended to after index build, reallocation invalidates stored pointers. Two-pass build (finalize slice, then index) eliminates this.

### Option 2: Indices (`[]int`)

Survives reallocation — useful for long-lived caches. Not needed here (request-scoped, slice never mutates after build). Every access requires carrying the source slice: `nodes[idx[i]].ID` instead of `n.ID`.

### Option 3: Value copies (`[]WorkloadNode`)

Looks safe, isn't. Two problems:

**`WorkloadNode` is not comparable.** It contains `map[string]string` and `[]StatusKey` — Go does not allow `==` on structs with map or slice fields:

```go
nodeA == nodeB  // compile error: struct containing map/slice is not comparable
```

So intersection cannot use `WorkloadNode` as a map key at all. Only option is string key:

```go
// values: forced to use string key
seen := map[string]bool{}
seen[node.ID] = true  // string hash per probe

// pointers: use address directly  
seen := map[*WorkloadNode]bool{}
seen[node] = true  // pointer hash, no string involved
```

**Isolation is illusory.** `Labels` and `Statuses` share backing storage across copies — mutating them on a copy corrupts the source. Real isolation needs a deep copy.

### Recommendation

Pointers. Contract: build nodes fully formed in one pass, treat as read-only after.

### Pointer footguns

**Appending after index build** — realloc invalidates all stored pointers:

```go
idx := buildLabelIndex(nodes)
nodes = append(nodes, newNode)  // realloc → pointers in idx now dangle, silent corruption
```

Guarded by two-pass pattern. Never build the index before the slice is final.

**Mutating through a pointer** — `Labels` and `Statuses` are reference types; writing through any pointer corrupts all holders:

```go
node := indexLabelMatch(selector, idx)[0]
node.Labels["x"] = "y"  // mutates the original — visible to every other caller holding this pointer
```

Nothing enforces immutability. If enrichment or normalization is added after index build, it must copy-on-write rather than mutate in place.

---

## Why two passes (build nodes, then build index)

Inline build during node construction looks tempting — saves one N-loop. **Don't do it.**

Problem: `append(nodes, ...)` may grow the slice past capacity → new backing array → previously stored `*WorkloadNode` pointers point at the freed array. Silent corruption, hard to debug.

```go
// BROKEN — pointer becomes stale on next append
nodes = append(nodes, WorkloadNode{...})
n := &nodes[len(nodes)-1]
for k, v := range n.Labels {
    idx[labelKey(k, v)] = append(idx[labelKey(k, v)], n)  // n may dangle later
}
nodes = append(nodes, anotherNode)  // realloc → n now invalid
```

Two-pass keeps it safe:

```go
nodes := buildAllNodes(...)        // slice finalized, no more appends
idx := buildLabelIndex(nodes)      // pointers stable from here on
```

Cost of second pass: ~1µs for 100 nodes. Negligible vs the 50-200ms k8s API calls dominating the request.

**Alternatives considered:**

- Preallocate with known max capacity (`make([]WorkloadNode, 0, max)`) — works but adds bookkeeping for trivial gain
- Store `[]int` indices instead of pointers — survives growth, adds indirection per lookup, awkward for callers expecting `*WorkloadNode`

Two-pass wins on safety and simplicity.

---

## Files to change

### `internal/graph/build-nodes.go`

- Add `labelKey(k, v string) string` helper
- Add `buildLabelIndex(nodes []WorkloadNode) LabelIndex` constructor
- Add `(LabelIndex).Match(selector) []*WorkloadNode` method
- Update `findNodeByLabel` call at line 70 (`buildWorkloadNodesForNS` services loop) — pass index instead of nodes slice
- Update `getSourceNodes` at line 119-127 — call `idx.Match(policy.Spec.PodSelector.MatchLabels)`
- Delete `findNodeByLabel` once all call sites migrated

### `internal/graph/build-edges.go`

- Update `generateEdges` signature: take `map[string]LabelIndex` (ns → index) instead of `map[string][]WorkloadNode`
- Replace `findNodeByLabel(peer.PodSelector.MatchLabels, nodes[srcNs])` at line 84 → `labelIdx[srcNs].Match(peer.PodSelector.MatchLabels)`
- Replace `findNodeByLabel(peer.PodSelector.MatchLabels, nsNodes)` at line 117 → `labelIdx[ns].Match(peer.PodSelector.MatchLabels)`
- Same return type change (slice of pointers vs slice of values — adapt edge construction)

### `internal/graph/workload.go`

- `BuildGraph` builds label index per ns after fetching nodes (line ~48 area)
- Pass `map[string]LabelIndex` to `buildEdges` (line 76)
- `getNodePolicies` loop at lines 60-68 can also use the index — flip the loop: for each policy, lookup matching nodes via index, then attach statuses (replaces N×P scan with P×L probes)

### Return type adaptation

Current `findNodeByLabel` returns `[]WorkloadNode` (values). Proposed `Match` returns `[]*WorkloadNode` (pointers). Callers must adapt — either:
- Dereference pointers when constructing edges (`PolicyEdge{Source: n.ID, ...}` works either way since `.ID` access works on pointer)
- Or change `WorkloadNode` slices throughout to pointer slices

Pointer approach saves struct copies, keeps mutations visible. Recommend pointers end-to-end.

---

## Non-goals (not in this change)

- Concurrent index build — not needed, per-ns build is fast
- Cross-namespace global index — selectors are always per-ns scoped
- Cache index across requests — request-scoped, rebuilt each `/api/graph`
- Fast path for `app.kubernetes.io/name` — measured savings ~300ns/request, not worth complexity

---

## Validation

- Existing `nodes_test.go` and `edge_test.go` cover label matching cases
- Add unit tests for `LabelIndex`:
  - Empty selector → nil
  - Single label match → exact bucket
  - Multi-label intersection → only nodes in all buckets
  - No bucket exists → nil (not empty slice)
- Benchmark before/after with synthetic 100-node × 50-policy fixture
