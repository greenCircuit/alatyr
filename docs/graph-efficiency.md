# Graph efficiency improvements

Three inefficiencies in the current graph-building pipeline, ordered by impact.

---

## 1. `findNSNode` — repeated O(n) scan inside loops

`findNSNode` scans `[]WorkloadNode` every time it needs the namespace node.
Called inside nested loops in `edge.go:99,107` and `workload.go:93` — once per policy × per peer.

**Fix:** split namespace nodes into their own map at build time:

```go
type NSIndex struct {
    workloads map[string][]WorkloadNode  // ns → workload nodes only
    nsNodes   map[string]*WorkloadNode   // ns → its namespace node (O(1) lookup)
}
```

---

## 2. `findNodeByLabel` — O(nodes × labels) per call, hot in every policy peer loop

Called at `edge.go:84,117` and `workload.go:41` — once per peer in every rule in every policy.
With P policies × R rules × E peers, that's many full node-list scans.

**Fix:** inverted label index per namespace.

Leaf is `[]*WorkloadNode` (slice, not single node) — multiple nodes can share the same label value.

```go
// ns → labelKey → labelValue → nodes
type LabelIndex map[string]map[string]map[string][]*WorkloadNode
```

**Build once after constructing nodes:**

```go
func buildLabelIndex(nodesByNS map[string][]WorkloadNode) LabelIndex {
    idx := LabelIndex{}
    for ns, nodes := range nodesByNS {
        idx[ns] = map[string]map[string][]*WorkloadNode{}
        for i := range nodes {
            node := &nodes[i]
            for k, v := range node.Labels {
                if idx[ns][k] == nil {
                    idx[ns][k] = map[string][]*WorkloadNode{}
                }
                idx[ns][k][v] = append(idx[ns][k][v], node)
            }
        }
    }
    return idx
}
```

**Query — intersect per label key:**

```go
func (idx LabelIndex) find(ns string, selector map[string]string) []*WorkloadNode {
    var result []*WorkloadNode
    first := true
    for k, v := range selector {
        candidates := idx[ns][k][v]
        if first {
            result = candidates
            first = false
            continue
        }
        set := make(map[*WorkloadNode]bool, len(candidates))
        for _, n := range candidates {
            set[n] = true
        }
        filtered := result[:0]
        for _, n := range result {
            if set[n] {
                filtered = append(filtered, n)
            }
        }
        result = filtered
    }
    return result
}
```

Empty selector (catch-all) is already handled by `isCatchAll` before `findNodeByLabel` is called — no change needed there.

---

## 3. Sequential k8s API calls

`workload.go:20-27` and `workload.go:44-53` fetch each namespace one at a time.
Calls are independent — run them concurrently with `errgroup`.

---

## Proposed combined structure

Replace `nodesByNS map[string][]WorkloadNode` with a single index built once in `BuildGraph`:

```go
type NSIndex struct {
    workloads  map[string][]WorkloadNode
    nsNodes    map[string]*WorkloadNode
    labelIndex map[string]map[string]map[string][]*WorkloadNode
}
```

Pass it into `buildEdges`, `getSourceNodes`, and `generateEdges`.
All three hot paths become direct lookups; `findNSNode` and `findNodeByLabel` can be deleted.
