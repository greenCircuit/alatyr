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

## Mapping nodes to their policies

To compute status keys, each node needs the set of `NetworkPolicy` objects that select it. Policies are always namespace-scoped — a policy in namespace `foo` can only select pods in `foo` (k8s enforces this). `getSourceNodes` already relies on this invariant.

### Strategy 1 — Scan all policies per node (`getNodePolicies`) ✅ implemented

For each node, iterate all policies and check if the policy selects it.

```go
func getNodePolicies(node WorkloadNode, policies []networkingv1.NetworkPolicy) []networkingv1.NetworkPolicy {
    var matches []networkingv1.NetworkPolicy
    for _, policy := range policies {
        if utils.IsLabelMatch(policy.Spec.PodSelector.MatchLabels, node.Labels) {
            matches = append(matches, policy)
        }
    }
    return matches
}
```

Called directly in the status loop in `workload.go` — node is already in scope so no pre-processing needed.

**Complexity:** O(nodes × policies × label-match cost)

**Pro:** simple, node already in scope at call site, no extra data structure.

**Con:** redundant label matching at scale.

**Note on inverting (Strategy 3):** pre-indexing policies → nodes via `getSourceNodes` reorders the work but does the same total label comparisons — `nodes × policies` either way. No complexity saving. Not worth adding `buildPolicyIndex` unless `getNodePolicies` is called multiple times per node in the future.

### Strategy 2 — Policy map + edge lookup

Build `map[string]networkingv1.NetworkPolicy` keyed by `"namespace/name"` once. `PolicyEdge` already carries `Namespace` + `PolicyName`. Per node: collect edges where `source == node.ID`, deduplicate policy keys, look up each policy in the map O(1).

**Con:** only covers policies that produced at least one edge — deny-all policies (no edges) are missed.

---

## Recommendation

**Current implementation uses Strategy 1** — `getNodePolicies` called per node in `workload.go`. Correct and simple for k8s cluster scale (tens of policies). No reason to change unless `getNodePolicies` needs to be called multiple times per node.

---

## Namespace node status keys

Namespace nodes (`ns-<name>`) represent the entire namespace, not a specific workload. Two new keys apply only to them:

| Key | Meaning |
|-----|---------|
| `ns-ingress` | at least one catch-all policy (empty `podSelector`) in this namespace restricts ingress |
| `ns-egress` | at least one catch-all policy (empty `podSelector`) in this namespace restricts egress |

These replace what would otherwise be self-referencing arrows: when a catch-all policy selects the namespace node as source and a peer rule targets pods in the same namespace, the edge source and target collapse to the same node. A status badge is clearer than a loop arrow.

### Detection logic

Scan all policies in the namespace. For each policy where `podSelector` is catch-all (empty `MatchLabels`, zero `MatchExpressions`), check `PolicyTypes`:

```
for each policy in namespace:
    if not catch-all podSelector → skip
    if PolicyTypes includes Ingress (or PolicyTypes is unset) → hasNSIngress = true
    if PolicyTypes includes Egress (or PolicyTypes is unset and Egress rules present) → hasNSEgress = true
```

The unset-PolicyTypes rule mirrors k8s semantics: omitting `PolicyTypes` locks ingress by default; egress is locked only if egress rules are present.

### Code split

`buildNSStatusKeys(policies []NetworkPolicy) []StatusKey` — standalone function, only called for `NodeTypeNamespace` nodes. Does not call `buildStatusKeys` or share logic with it; the two functions answer different questions.

In `workload.go`, branch on node type:

```go
for i := range nodes {
    if nodes[i].Type == NodeTypeNamespace {
        nodes[i].Statuses = buildNSStatusKeys(policies)
    } else {
        matchPolicies := getNodePolicies(nodes[i], policies)
        nodes[i].Statuses = buildStatusKeys(matchPolicies)
    }
}
```

### Alternative — single function with internal branch

Pass the node into one combined function and branch inside:

```go
func computeStatuses(node WorkloadNode, policies []networkingv1.NetworkPolicy) []StatusKey {
    if node.Type == NodeTypeNamespace {
        return buildNSStatusKeys(policies)
    }
    return buildStatusKeys(getNodePolicies(node, policies))
}
```

Caller becomes one line:

```go
for i := range nodes {
    nodes[i].Statuses = computeStatuses(nodes[i], policies)
}
```

**Why not chosen:** the two code paths answer fundamentally different questions — NS nodes look at catch-all policies across the whole namespace; workload nodes look at policies that select them by label. Merging them behind one name obscures that distinction and makes unit tests harder: you can't test the NS path without constructing a node+policies combo, and same for the workload path. The external branch is 3 extra lines but keeps both functions pure and independently testable.

**When this alternative is preferable:** if more node types are added later that each need their own status logic — the single dispatch function scales better than an ever-growing `if/else` chain at the call site.

---

## Refactoring buildStatusKeys

`buildStatusKeys` mixes two concerns: scanning policies into boolean signals, then mapping those booleans to `[]StatusKey`. Split them.

### Step 1 — extract a traits struct

```go
type policyTraits struct {
    egressLocked       bool
    ingressLocked      bool
    hasInternetEgress  bool
    hasInternetIngress bool
    hasCrossNS         bool
}
```

### Step 2 — analyzeTraits(policies) policyTraits

Pure scan: iterate policies, fill the struct. No StatusKey logic here.

```go
func analyzeTraits(policies []networkingv1.NetworkPolicy) policyTraits {
    var t policyTraits
    for _, policy := range policies {
        // fill t.egressLocked, t.ingressLocked, t.hasInternetEgress, etc.
    }
    return t
}
```

### Step 3 — buildStatusKeys becomes a thin mapper

```go
func buildStatusKeys(policies []networkingv1.NetworkPolicy) []StatusKey {
    t := analyzeTraits(policies)
    var result []StatusKey
    if !t.egressLocked || t.hasInternetEgress  { result = append(result, StatusInternetEgress) }
    if !t.ingressLocked || t.hasInternetIngress { result = append(result, StatusInternetIngress) }
    if t.egressLocked && t.ingressLocked && !t.hasInternetEgress && !t.hasInternetIngress {
        result = append(result, StatusIsolated)
    }
    if t.hasCrossNS { result = append(result, StatusCrossNamespace) }
    return result
}
```

### Reuse in buildNSStatusKeys

`buildNSStatusKeys` can call `analyzeTraits` on the catch-all-filtered policy slice, then read only `egressLocked` and `ingressLocked`:

```go
func buildNSStatusKeys(policies []networkingv1.NetworkPolicy) []StatusKey {
    var catchAll []networkingv1.NetworkPolicy
    for _, p := range policies {
        if isCatchAll(p.Spec.PodSelector.MatchLabels, len(p.Spec.PodSelector.MatchExpressions)) {
            catchAll = append(catchAll, p)
        }
    }
    t := analyzeTraits(catchAll)
    var result []StatusKey
    if t.ingressLocked { result = append(result, StatusNSIngress) }
    if t.egressLocked  { result = append(result, StatusNSEgress) }
    return result
}
```

`analyzeTraits` is the only shared piece. Both callers stay pure functions with no side effects.

---

## Custom status keys (config-driven)

Default keys (`internet-ingress`, `isolated`, etc.) are hardcoded because they derive purely from standard k8s policy semantics — no cluster-specific knowledge needed. Custom keys fire based on whether specific peers are present or absent in a node's policies, defined in `defaultConfig.yaml` (path overridable via env var).

### Why implement

Default keys answer generic questions ("is this node isolated?"). Custom keys answer cluster-specific questions ("does this workload have access to the API server?", "is DNS egress allowed?", "is istio-ingress the only ingress source?"). Without custom keys, these checks require manual policy inspection per cluster.

### Config shape

```yaml
customRules:
  ingress:
    - name: istio-ingress
      status: present
      policies:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: istio-ingress
          podSelector:
            matchLabels:
              app: istio-ingress
  egress:
    - name: dns-missing
      status: missing
      policies:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
          port: 53
        - port: 53
```

- `status: present` — emit key if any peer in any matched policy satisfies one of the listed patterns
- `status: missing` — emit key if direction is locked AND none of the patterns are satisfied; if direction is not locked the pod can reach the target anyway so the key is skipped

Multiple entries under `policies` are OR'd — any one match is sufficient.

### Peer matching

A config pattern matches a policy peer when all specified fields agree (AND):

```go
func peerMatchesPattern(peer networkingv1.NetworkPolicyPeer, ports []networkingv1.NetworkPolicyPort, pattern ConfigPolicyPattern) bool {
    if pattern.NamespaceSelector != nil {
        if peer.NamespaceSelector == nil { return false }
        if !utils.IsLabelMatch(pattern.NamespaceSelector.MatchLabels, peer.NamespaceSelector.MatchLabels) { return false }
    }
    if pattern.PodSelector != nil {
        if peer.PodSelector == nil { return false }
        if !utils.IsLabelMatch(pattern.PodSelector.MatchLabels, peer.PodSelector.MatchLabels) { return false }
    }
    if pattern.Port != 0 {
        if !portsInclude(ports, pattern.Port) { return false }
    }
    return true
}
```

Called once per peer inside the existing peer loop in `buildStatusKeys` — no new loop structure at the call site.

### Detection: present vs missing

`present` rules accumulate matches; `missing` rules use the inverse — pre-populate a set of all missing rule names, delete when a match is found:

```go
// before policy loop
unmatched := map[string]bool{}
for _, rule := range config.Egress {
    if rule.Status == "missing" {
        unmatched[rule.Name] = true
    }
}

// inside peer loop — when peer matches a missing rule's pattern
delete(unmatched, ruleName)

// after all policies scanned
for name := range unmatched {
    if egressLocked {
        result = append(result, StatusKey(name))
    }
}
```

`present` matched names accumulate in a separate set, emitted unconditionally after the loop.

### Tradeoffs

**Pro**
- Cluster-specific checks without code changes — ops teams edit YAML, not Go.
- Evaluated inside existing peer loops in `buildStatusKeys` — no extra pass over policies.
- `missing` detection is O(config rules) per node, not O(policies²).

**Con**
- Config patterns only match on `namespaceSelector`, `podSelector`, and `port` — cannot express arbitrary conditions (e.g. "egress locked AND specific CIDR"). CEL or a richer expression language would be needed for that.
- Config must be kept in sync with the cluster (e.g. if istio-ingress namespace is renamed, the rule silently stops matching).
- Frontend must receive custom key metadata (symbol, color, title) from the backend — either via `/api/config` or bundled into `/api/graph` response — since it can no longer hardcode `STATUS_CFG`.
