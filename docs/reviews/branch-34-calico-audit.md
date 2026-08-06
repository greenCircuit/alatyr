# Branch 34 — Calico engine audit

Scope: `internal/policy/calico/` (evaluate.go, buildRules.go, buildPolicyStatus.go, calico.go, utils.go, calico_test.go) plus shared types in `internal/models/rule.go`, `internal/models/evaluation.go`.

Compared against shipped engines: `internal/policy/k8spolicy/` and `internal/policy/istio/`.

Reviewers: `backend-sre` (runtime/SRE lens) + maintainability lens (general-purpose agent).

---

## Critical — blocks shipping

### 1. `evaluate.go:139-141` — engine inert
`getPolicies` returns `nil, nil` unconditionally. Full pipeline (sort → resolve → status) walks empty slice. All resolve logic has never seen a real Calico CRD in production. Test at `calico_test.go:12-29` constructs `globalPolicy` directly with hand-rolled `selectorMatch: func(...) bool { return true }` — zero coverage of the parse boundary that produces those matchers when wire-up lands.

**Fix:** wire `k8s.KubernetesClient.GetGlobalNetworkPolicies` via Calico client-go; normalize CRD → `globalPolicy` (order via `normalizeOrder`, selectors via `parseSelector`).

### 2. `utils.go:75-82` — `parseSelector` accepts only `""` and `"all()"`
Every real Calico policy uses `k == 'v'` / `has(label)` / set expressions. On wire-up, first non-trivial policy hard-errors. If error propagates (matches k8spolicy pattern at `evaluate.go:73-74`), one bad policy poisons the whole engine batch → graph loses all calico edges cluster-wide. If swallowed, workloads governed by real selectors silently look unenforced. Either mode = 3am page.

**Fix:** implement real selector parsing OR gate engine registration on parseSelector completeness.

### 3. `evaluate.go:79-84` — Unenforced marker lands in `.Allow` bucket
Zero-value `Action == ActionAllow`. Downstream `renderEdges` groups by action → CoverageUnenforced rule renders as phantom allow edge with empty peer. Detail-panel "policies affecting this workload" will show a phantom allow.

**Fix (pick one):**
- Add `ActionUnenforced` action.
- Add third bucket on `DirectionRules` (e.g. `Unenforced []Rule`).
- Don't emit a Rule; signal via `PolicyStatus` only (k8spolicy path).

Decide before frontend consumes the field.

### 4. `evaluate.go:32` — `ctx` dropped at `getPolicies(_ context.Context)`
No deadline, no cancellation on slow API server. Client hang stalls the whole engine.

**Fix:** wire `ctx` through the client call before shipping.

### 5. `buildRules.go:56-71` — `isPass` `break` will be wrong when tiers land
Current code: `break` exits inner rule loop, outer `for _, gp := range ordered` advances. Correct today (no tier concept). When tiers land, Pass at last rule of last policy in a tier must fall through to next tier's default action, not next policy in same tier. No tier boundary check exists.

**Fix:** file follow-up ticket now; wrong-answer silent bug if tiers ship without it.

---

## Performance — real-cluster risk

### 6. `evaluate.go:47-55` — O(ns × workloads × policies × rules × buckets), no memoization
At 50 ns × 500 workloads × 200 GNPs × ~5 rules × ~3 CIDRs → inner loop ~75M/graph-fetch. `resolveDirection` (`buildRules.go:26-35`) rebuilds `destinationBuckets` and re-walks `resolveBucket` for every workload. Bucket set is identical across all workloads selected by the same policy set.

Also: `netsCover` (`utils.go:40-55`) calls `policy.CidrContains` (string parse per call) inside the innermost loop.

**Fix:**
- Memoize per unique selecting-policy-set.
- Precompute CIDRs into `net.IPNet` at normalize time.

Defer until measured on real cluster — premature otherwise.

---

## Maintainability

### 7. `evaluate.go:79-84` — Unenforced logic misplaced
`anyGoverns` + `unenforcedRule` are status-shaping concerns. Living in `resolveWorkload` hides the "no opinion" contract when reading `buildRules.go`.

**Fix:** move to `buildPolicyStatus.go` (or new `coverage.go`); expose `applyUnenforcedMarkers(&nodeRules, selecting)` — orchestrator stays 3 lines.

### 8. `evaluate.go:94-101` — ns-flatten boilerplate duplicated across engines
Same 4-line append pattern in istio (`evaluate.go:62-69`) and k8s (`buildAllowRulesByNs`).

**Fix:** extract `policy.FlattenNodeRulesByNs(nodeID, ns, NodeRules, *EvaluationResult)` in `internal/policy/`. Stops `DirectionRules` shape leaking into every orchestrator.

### 9. `EvaluationResult` init inconsistent across three engines
Calico initializes 6 maps; k8s omits `DenyByNs`+`GlobalPolicies`; istio omits `Nodes`+`GlobalPolicies`. Nil-map panic one refactor away.

**Fix:** `models.NewEvaluationResult()` constructor seeds every map; every engine calls it. Doc-comment on `EvaluationResult` marks optional vs required fields.

### 10. Naming — `globalPolicy`, `resolvedEdge`
`globalPolicy` too generic — next-year reader expects a shared abstraction across engines (contrast with the calico-namespaced `calicoRule`). `resolvedEdge` isn't an edge; it's the first-match verdict per destination bucket (no src/dst yet).

**Fix:** `globalPolicy` → `normalizedGNP` or `gnp` (mirrors CRD kind). `resolvedEdge` → `bucketVerdict`.

### 11. Duplicated ADR pointers
Three separate `SEAM` / `ADR 0005` TODO comments describe the same "engine inert until libcalico wired" state (`utils.go:75`, `evaluate.go:139`, plus scattered in `calico.go`). The `utils.go:16` comment on `normalizeOrder` is a question, not documentation.

**Fix:** collapse to one package-level doc-comment on `calico.go` listing seams. Drop per-function ADR pointers. File ticket for the `normalizeOrder` question; delete comment.

### 12. `buildRules.go:73-78` — 6-line docstring on `buildRulesForWorkload`
Violates project rule (≤2 lines, non-obvious WHY only).

**Fix:** trim to: "catchAllCIDR skips peer-node synthesis; Coverage marker carries the direction-wide semantic."

### 13. `calico_test.go:12-29` — `buildTestPolicy` 5-positional-arg signature
`(name, doesEgress, doesIngress, egress, ingress)` will bite next test author.

**Fix:** options struct or fluent builder.

---

## Not-a-problem (called out to prevent churn)

- First-match walk lives correctly in `buildRules.go`.
- Status derivation cleanly separated in `buildPolicyStatus.go`.
- `netsCover`, `selectsWorkload` in `utils.go` are correctly-scoped pure helpers.
- Recent refactor (`buildRulesForWorkload` returns `DirectionRules`) removed the redundant re-walk loop cleanly.
- Testability is good — pure `resolveWorkload` takes `[]globalPolicy` + node directly, no client needed. Keep as engines grow.

---

## Priority action order

1. Fix `parseSelector` OR gate wire-up on completeness — **blocker for shipping**.
2. Decide unenforced-marker contract — **blocker for UI consumer**.
3. Extract `NewEvaluationResult()` — cheap, kills a whole class of nil-panics.
4. Wire `ctx` through `getPolicies`.
5. Move `anyGoverns`/`unenforcedRule` into `buildPolicyStatus.go`.
6. Extract `FlattenNodeRulesByNs` shared helper.
7. Perf memoization — defer until measured on real cluster.
8. File tier follow-up ticket for Pass semantics.
