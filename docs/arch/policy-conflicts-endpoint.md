# Policy Conflicts Endpoint — v1 (DNS egress only)

## What this is

A new backend endpoint `GET /api/policy-conflicts` that returns a list of policy misconfiguration findings across all workloads.

v1 scope is intentionally tiny: **one check — DNS egress missing**. A workload has egress locked down by some policy but no rule allows `UDP/53` to `kube-system`. That workload will fail DNS resolution in prod.

Everything else (ingress checks, ALLOW+DENY conflicts, mesh problems) is v2.

---

## Frame — Issue vs Status Key

Status keys (`StatusKey` enum in `internal/models/statusKeys.go`) already tell you what's true for each workload: "has internet egress", "has LAN ingress", etc. Great for a badge on the node.

But an issue needs more than a boolean flag:
- A **message** (what to fix)
- A **severity** (warn / error)
- **Evidence** (which policies are involved)

The enum can't carry that. So `Issue` is a separate type. The two live side by side:
- `StatusDnsEgress` (enum) — node badge, "workload can reach DNS"
- `Issue{Type: dns-egress-missing, ...}` — actionable finding

Both are produced in the same engine pass — no extra loop.

---

## Where the work happens (and why)

### Decision 1 — Engine emits issues, not the store layer

**Chosen:** each engine (`k8spolicy`, `istio`) emits `Issue` values during its existing status-key pass, into `EvaluationResult.Issues`. The store layer just flattens.

**Rejected:** deriving issues in the store layer by diffing status keys.

**Pros of engine-side emission**
- Engine is already iterating every workload's egress rules — cheapest place to notice "no DNS allow".
- One source of truth. No re-derivation drift.
- Store-side code is trivial (concat maps), less to test.
- Sparse output — only workloads with findings show up in the map. Endpoint doesn't scan every workload.

**Cons**
- Engine now knows about `Issue` severity/message. Some might argue that's a presentation concern. Trade-off accepted because the engine is the only thing that has the full context to describe the finding accurately.
- Adding a new check means editing every engine that could produce it. Same cost as adding a new status key today.

### Decision 2 — Status key `StatusDnsEgress` stays alongside

**Chosen:** engine sets `PolicyStatus.DnsEgress = true/false` in the same pass, `DeriveStatusKeys` maps that to `StatusDnsEgress`. Node still shows a badge.

**Why:** the graph UI already shows status keys as badges. Users get the presence/absence signal at a glance without opening the issues panel. Cheap — one more bool field, one more enum entry.

### Decision 3 — `ByNode` index on `EvaluationResult`

Current `EvaluationResult` has `AllowByNs` and `DenyByNs` (rules grouped by namespace). Good for "give me all rules in ns X". Bad for "give me every rule touching workload W across the whole cluster".

**Chosen:** add `ByNode map[nodeID][]Rule`. Each rule is filed under **both** its `SrcID` and its `DstID`. One lookup returns everything relevant to a workload.

**Rejected:** split `BySrc` + `ByDst`.

**Pros of omnibus `ByNode`**
- Any endpoint that asks "what does policy say about W" is one map lookup. Zero filtering.
- The dominant future access pattern is per-workload views (issues panel, blast radius, detail sidebar). This is the shape they all want.
- No knowledge of direction needed at read time.

**Cons**
- Each rule is stored twice (once under `SrcID`, once under `DstID`). Storage overhead is small — `Rule` is a modest struct, cluster edge counts are in the thousands, not millions.
- Direction-specific queries (egress-only) need one filter pass. Acceptable — DNS check is the only one that does this and it's a linear scan over a small slice.
- Self-loop rules (same src and dst) appear twice under the same key. Consumer dedupes if listing "unique rules". Documented on the field.

### Decision 4 — Shared `IndexByNode` helper

Both engines have to populate `ByNode` correctly. If each engine rolls its own indexer, they drift.

**Chosen:** one helper `IndexByNode(allow, deny []Rule) map[string][]Rule` in `internal/policy/`. Both engines call it at the tail of their `Evaluate`. One code path, no drift.

### Decision 5 — Per-engine issue rows, no cross-engine merge

**Chosen:** one `Issue` per engine per workload. If k8s NetworkPolicy is missing DNS but Istio ALLOW has it, both engines are asked, and the k8s issue still fires.

**Rejected:** merged effective-view (like `IsNodesReachable` does for the reachability endpoint).

**Why not merge:** reachability answers a single bit ("can A reach B"). "Which YAML do I edit to fix DNS" is not a single bit. If we merge and only report when both engines are missing DNS, the user never learns their k8s NetworkPolicy is broken until they turn Istio off. That's exactly the class of quiet lie we want to avoid.

The frontend can group by workload if it wants a rolled-up view. Client-side concern.

### Decision 6 — Config left untouched

`internal/config/defaultConfig.yaml` has a `customRules` section with an unwired `dns-missing` rule and schema in `internal/config/config.go`. Nothing reads it today.

**Chosen:** leave it as a stub. Compile in the DNS check the same way `StatusApiServerEgress` is compiled in.

**Rejected for now:** wiring the config-driven evaluator.

**Pros of hardcoded**
- No config schema surface to keep correct (CIDR peers, protocol semantics, "no selectors means any peer", etc.).
- Consistent with the existing pattern — every status key today is engine-hardcoded.
- No extra loop pass at request time.

**Cons**
- Kube-dns / port 53 / kube-system are compiled constants. Any cluster with weird DNS (custom port, cluster-local DNS proxy) needs a code change.
- Adding a new check means engine code, not YAML.

**When to revisit:** the first time an operator asks for a cluster-specific rule ("my payment-svc must egress to prod-vault on 8200"). Until then, config is speculative abstraction and nobody wired it — that's a signal.

---

## Detection signal (v1)

**A workload has DNS egress when:** the engine finds at least one allow rule where
- `SrcID == workload.ID`
- `Direction == DirectionEgress`
- Peer selector matches namespace `kube-system` (label `kubernetes.io/metadata.name: kube-system`)
- Port UDP/53 is present (or `AllPorts == true`)

No pod-label check on the peer — avoids kubeadm (`k8s-app=kube-dns`) vs k3s (`k8s-app=kube-dns` too, but different distros differ) drift.

**Known false positive (documented in the issue message):** clusters that allow DNS via CIDR (`ipBlock: 10.43.0.10/32` = kube-dns service IP) instead of selector will be flagged as missing. Extending `PolicyPeer` schema to accept CIDR is a v2 item.

**Known false positive:** workloads using node-local-dns (`169.254.20.10`) or Istio's DNS proxy bypass kube-dns entirely, so their egress policy legitimately doesn't include kube-system. No signal in NetworkPolicy to detect this — will be flagged. Documented.

---

## Type additions

`internal/models/statusKeys.go`
```go
const StatusDnsEgress StatusKey = "dns-egress"

type PolicyStatus struct {
    // ...existing fields...
    DnsEgress bool
}
```

`internal/models/issues.go` (replace current stub — it has an unexported `message` field and two constants sharing the value `"no dns"`)
```go
type IssueType string
const (
    IssueDnsEgressMissing IssueType = "dns-egress-missing"
)

type IssueSeverity string
const (
    SeverityWarn  IssueSeverity = "warn"
    SeverityError IssueSeverity = "error"
)

type Issue struct {
    Type      IssueType     `json:"type"`
    Severity  IssueSeverity `json:"severity"`
    NodeID    string        `json:"nodeId"`
    Namespace string        `json:"namespace"`
    Engine    string        `json:"engine"`
    Message   string        `json:"message"`
    Evidence  []PolicyRef   `json:"evidence"`
}
```

`internal/models/evaluation.go`
```go
type EvaluationResult struct {
    AllowByNs      map[string][]Rule
    DenyByNs       map[string][]Rule
    PolicyStatuses map[string]PolicyStatus
    NodePolicies   map[string][]PolicyRef
    ByNode         map[string][]Rule   // rule filed under both SrcID and DstID
    Issues         map[string][]Issue  // sparse — only workloads with findings
}
```

---

## Engine changes (both k8spolicy and istio)

The existing status-computation pass already iterates each workload's egress rules to decide `EgressLocked`, `InternetEgress`, etc. Add two things in the same loop:

1. Set `policyStatus.DnsEgress = <did we see a DNS allow>` while iterating.
2. If `EgressLocked && !DnsEgress`, append to `issues[node.ID]`:
   ```
   Issue{
       Type: IssueDnsEgressMissing,
       Severity: SeverityError,
       NodeID: node.ID,
       Namespace: node.Namespace,
       Engine: <engine name>,
       Message: "workload egress is locked but no rule allows UDP/53 to kube-system (ipBlock-based allows are not detected in v1)",
       Evidence: <NodePolicies filtered to Direction == DirectionEgress>,
   }
   ```

**Evidence filter:** `PolicyRef.Direction` field exists (`internal/models/rule.go:47`). Use it so the operator opens the right YAML — an ingress policy that happens to select the workload isn't relevant to a missing egress rule.

Then at the tail of each engine's `Evaluate`, call the shared `IndexByNode` helper to populate `ByNode`.

---

## Store layer

Replace the untracked stub in `internal/store/issues.go` with a flatten-only function:

```go
func FindIssues(data *models.Cache) []models.Issue {
    var out []models.Issue
    for _, eval := range data.EvaluationResults {
        for _, issues := range eval.Issues {
            out = append(out, issues...)
        }
    }
    return out
}
```

Delete the `DnsIssues` / `MeshIssuer` / `PolicyIssues` stub functions — they represented the old "store computes findings" model.

---

## Endpoint

New file `internal/api/issues.go`, handler mirrors `handleGraph` in `internal/api/network-policy.go`:
- Grab `s.mu.RLock()`.
- Resolve namespaces (same rule as `/api/graph` — no param means all).
- `s.store.PopulateCache(...)`.
- Return `store.FindIssues(s.cache)` as JSON.

One line in `internal/api/api.go:30`:
```go
e.GET("/api/policy-conflicts", s.handleIssues)
```

---

## Files touched

| File | Change |
|------|--------|
| `internal/models/evaluation.go` | Add `ByNode` and `Issues` fields |
| `internal/models/statusKeys.go` | Add `StatusDnsEgress` and `PolicyStatus.DnsEgress` |
| `internal/models/issues.go` | Replace stub with `Issue`, `IssueType`, `IssueSeverity` |
| `internal/policy/statusKeys.go` | Map `DnsEgress` → `StatusDnsEgress` in `DeriveStatusKeys` |
| `internal/policy/index_by_node.go` (new) | `IndexByNode` helper |
| `internal/policy/k8spolicy/buildStatusKeys.go` | Set `DnsEgress`; emit Issue when locked+missing |
| `internal/policy/k8spolicy/evaluate.go` | Call `IndexByNode`; thread Issues into result |
| `internal/policy/istio/buildPolicyStatus.go` | Same as k8spolicy |
| `internal/policy/istio/evaluate.go` | Same as k8spolicy |
| `internal/store/issues.go` | Replace stub with `FindIssues` (flatten only) |
| `internal/api/issues.go` (new) | Handler |
| `internal/api/api.go` | One route registration line |

---

## Things worth double-checking during implementation

1. **CronJob coverage.** Confirm both engines populate `EgressLocked` for CronJob workloads, not just pods. If Istio's `generatePolicyStatusAssignment` filters to pods only, CronJobs will come back unlocked and be silently skipped from findings.
2. **Cache race.** Handler holds `s.mu.RLock()`. Confirm `PopulateCache` respects the write side or swaps `*Cache` atomically. Otherwise torn reads possible under concurrent requests.
3. **Evidence direction filter.** `NodePolicies[nodeID]` mixes ingress and egress selectors. Filtering before emit is the right call, but verify `PolicyRef.Direction` is actually populated by both engines when adding entries to `NodePolicies` — it might be empty in some paths.
4. **External nodes.** `EvaluationResult.PolicyStatuses` and `NodePolicies` should never contain external node IDs (empty ns, CIDR-derived), but defensive check in `FindIssues` if the maps are populated indirectly.

---

## How to test end-to-end

1. Start the backend: `KUBECONFIG=/etc/rancher/k3s/k3s.yaml go run main.go` (or `DEMO_MODE=true`).
2. Hit the endpoint: `curl 'http://localhost:8080/api/policy-conflicts' | jq`.
3. Expected: workloads with egress locked and no DNS allow return one issue per engine that flagged them. Most workloads return nothing.
4. Cross-check with `/api/graph`: the same workloads should be missing `StatusDnsEgress` from `StatusesBySource[<engine>]`.

### Unit tests to add

- `internal/policy/index_by_node_test.go` — empty, single rule, self-loop, cross-ns rules.
- `internal/policy/k8spolicy/build_status_keys_test.go` — extend with DNS present / absent fixtures. Assert both `PolicyStatus.DnsEgress` and `EvaluationResult.Issues` populated correctly.
- `internal/policy/istio/istio_test.go` — same shape.
- `internal/store/issues_test.go` — synthetic `Cache` with two engines flagging one workload → `FindIssues` returns two rows.

---

## Out of scope for v1

- Config-driven checks (`customRules` evaluator, CIDR support, protocol enforcement).
- Ingress-missing checks (istio-ingress port present, hbone, etc.).
- ALLOW+DENY overlap detection across engines.
- Mesh / L7 conflicts.
- Two-tier severity for "unlocked-in-restricted-ns" (SRE-recommended; noise vs. signal question deferred).
- Frontend rendering.
