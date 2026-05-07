# StatusKey Computation Approaches

StatusKeys are per-node badges computed from NetworkPolicy objects. Each node may earn multiple keys depending on what policies select it and what those policies permit or restrict.

## What each StatusKey requires

| Key | Needs |
|-----|-------|
| `no-policy` | does any policy's `podSelector` select this node? |
| `isolated` | node is selected by policy with both ingress+egress locked, no permissive rules |
| `internet-ingress` | selected policy has an `ipBlock` peer in an ingress rule |
| `internet-egress` | selected policy has an `ipBlock` peer in an egress rule |
| `dns-missing` | egress is locked, no rule opens port 53 |
| `cross-namespace` | selected policy has a `namespaceSelector` peer |
| `orphaned-selector` | a policy peer selector references pods that match no existing node |
| `kube-api-access` | selected policy has ipBlock covering the API server CIDR |
| `ingress-exposed` | node is targeted by a k8s `Ingress` resource (separate API call) |

All keys except `orphaned-selector` and `ingress-exposed` are node-centric: they depend on which policies select the node, then what those policies say.

`orphaned-selector` is policy-centric: it asks whether a peer selector inside a policy matches any node at all — independent of which node you're evaluating.

---

## Approach A — Naive: scan all policies per node

For every node, iterate the full policy list and check whether each policy selects it.

```
for each node:
    for each policy:
        if policy selects node → inspect rules → accumulate status keys
```

**Complexity:** O(nodes × policies × rules)

**Pros**
- Trivial to implement: `getStatusKeys(node, allPolicies)` — no pre-processing.
- Easy to reason about: each node is self-contained.

**Cons**
- Redundant work: `getSourceNodes` label-matching runs N times per policy (once per node).
- At scale (many nodes, many policies), the quadratic node×policy factor dominates.
- In practice k8s clusters rarely have thousands of policies, so this rarely matters — but it's logically wasteful.

**When acceptable:** prototyping, clusters with <50 nodes and <50 policies.

---

## Approach B — Pre-index: invert policies → nodes once

Build a `map[nodeID][]NetworkPolicy` in a single pass using the already-written `getSourceNodes`. Then each node looks up only its own policies.

```
pass 1 (one time):
    for each policy:
        for each node getSourceNodes selects → index[node.ID] = append(policy)

pass 2:
    for each node:
        getStatusKeys(node, index[node.ID])
```

**Complexity:** O(policies × nodes_per_policy) for the index + O(nodes × avg_policies_per_node × rules) for status computation. Effectively linear in practice.

**Pros**
- No redundant label matching: each policy-node relationship is evaluated once.
- `getStatusKeys` stays a pure function — receives only the policies relevant to the node.
- Reuses `getSourceNodes` already written for edge building, no new label-match logic.
- Scales cleanly: adding more status keys doesn't increase the indexing cost.

**Cons**
- One extra pass over policies before node annotation.
- Slightly more code (the `buildPolicyIndex` helper).
- Index allocates a map; negligible at k8s scale but worth noting.

**Reasoning:** this is the right default. The indexing pass is the same work `buildEdges` already does implicitly — it just makes the node→policy relationship explicit and reusable.

---

## Approach C — Single pass with accumulation (piggyback on buildEdges)

During the existing `buildEdges` loop, accumulate per-node status signals as a side effect, then finalize statuses after the loop.

```
during buildEdges:
    when policy selects node → record signals on node (ipBlock seen, cross-ns seen, etc.)

after buildEdges:
    for each node: finalize StatusKeys from accumulated signals
```

**Complexity:** same as B — one pass over policies.

**Pros**
- No second pass; status signals are collected while building edges, which iterates policies anyway.
- Minimal extra allocations.

**Cons**
- Couples two separate concerns (edge topology and node status) in one function — harder to test independently.
- `buildEdges` currently returns `[]PolicyEdge` only; returning status signals too requires a new return type or mutation of node slices passed by reference.
- `orphaned-selector` and `no-policy` still need a separate post-processing step because they can only be determined after all policies are seen.
- Harder to extend: adding a new status key means modifying the edge-building loop.

**When useful:** if allocation pressure is a real concern and the codebase already mixes these concerns. Not recommended here — purity of `buildEdges` is more valuable.

---

## Approach D — Deferred / frontend computation

Don't compute statuses in the backend. Return raw policies alongside nodes/edges; let the frontend derive statuses from the graph it already has.

**Pros**
- Backend stays simpler: no status logic at all.
- Frontend can recompute statuses on filter changes without a round-trip.

**Cons**
- Duplicates graph traversal logic in TypeScript that already exists or will exist in Go.
- Frontend receives full policy objects — larger payload, tighter coupling to k8s API shapes.
- Status logic is harder to unit-test in the browser than in Go.
- Cytoscape re-layout already happens on filter changes; recomputing status there adds latency.

**Not recommended** for this project: the backend already has all the data and the right abstractions.

---

## Recommendation

**Use Approach B.**

It separates concerns cleanly, reuses existing helpers, keeps `getStatusKeys` a pure function (easy to unit-test), and is linear in the number of policy-node relationships. The `buildPolicyIndex` helper is ~10 lines and pays for itself immediately in readability.

`orphaned-selector` is the one exception: track it as a `set[policyName]` of policies with unmatched peers during `buildPolicyIndex`, then mark any node selected by such a policy with the badge.

```go
// in BuildGraph, after buildEdges:
policyIndex := buildPolicyIndex(nodesByNS, policies)
for ns := range nodesByNS {
    for i, node := range nodesByNS[ns] {
        nodesByNS[ns][i].Statuses = getStatusKeys(node, policyIndex[node.ID])
    }
}
```
