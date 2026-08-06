# Branch 34 — Calico GlobalNetworkPolicy Engine: Audit v2

**Branch:** `34-support-for-global-calico-polices`  
**Base:** `main`  
**Commits audited:** `517c4c2`, `4f8bb98`, `5ec5044` (fixong calico deny), `74b704e` (fix: update ui)  
**Date:** 2026-07-29  
**Build:** `GOTOOLCHAIN=auto go build ./...` — clean  
**Tests:** `go test ./internal/policy/calico/... ./internal/policy/... ./internal/graph/... ./internal/store/...` — all pass  

This document is self-contained. The v1 audit (`branch-34-calico-audit.md`) covered commits `517c4c2`+`4f8bb98`; this pass covers the full current HEAD. Each finding re-read from source at current line numbers.

---

## Summary of what changed since v1

Commits `5ec5044` and `74b704e` together delivered:

- `internal/thirdparty/calicoselector/` — real vendored Calico selector parser (Apache-2.0, copied from `github.com/projectcalico/calico@3d00673793c8`)
- `parseSelector` in `utils.go` now wires to the real parser — v1 Critical #2 resolved
- `getPolicies` in `evaluate.go` now calls `client.GetGlobalNetworkPolicies()` — v1 Critical #1 resolved
- `denyAllBaseline`/`unenforcedRule` in `evaluate.go` — v1 Critical #3 resolved via explicit `Coverage` constants
- `checkPeerSupported` in `decode.go` — label-scoped peer rejection added; fails loud, not silently
- `dropRestatedEdges` in `buildRules.go` — prevents phantom CIDR arrows when catch-all winner is first-match
- `collapseUniformRules` in `renderEngine.go` — namespace fan-out collapsed for `NamespaceWide` rules
- `calico_test.go` — significant test coverage added for resolution, coverage markers, enroll scenario, vendored parser
- `buildIssues.go` + `reachability.go` — calico-aware issue detection (`isInternalCidrAggregate`, `subsetAllowCidr`, `cidrNodeCoversPeer`, `policiesAllowingPeerSpecifically`)
- `InformerClient` + `DemoClient` wired for `GetGlobalNetworkPolicies` and `GetGlobalNetworkPolicyByName`
- `get-manifest.go` — calico case added to manifest endpoint

---

## Critical

None. All v1 Critical findings are resolved. Build is clean and tests pass.

---

## High

### PolicyRef missing Tier and Order — `internal/policy/calico/decode.go:64-68`
[FIXED]
- **Severity:** High
- **Why this severity:** `models.PolicyRef` has `Order *float64` and `Tier string` fields (added in `internal/models/rule.go:58-59`) with a comment saying "Calico precedence" and "Calico tier name". `toGlobalPolicy` never populates them: the `ref` literal at line 64 only sets `Source`, `Name`, `CreatedAt`. The internal `globalPolicy` struct carries `.order` and `.tier` (lines 55-56), but they are never transferred to the PolicyRef. Every culprit chip, detail panel row, and policy table row for a Calico policy will silently show nil/empty for both fields — the very fields that distinguish an order-500 enroll policy from an order-2000 default-deny. An operator debugging a first-match conflict has no way to see the deciding factor from the UI.
- **User impact:** Calico policies appear orderless in the detail panel and culprit list. An operator who added a higher-priority rule to a non-enrolled namespace would see two culprit chips with the same name/tier=`""` and no way to know which fired first. Not a data corruption, but the information is already in the struct — it just never makes it to the wire.
- **How it should be implemented instead:** In `toGlobalPolicy`, populate the `ref` fields before returning:
  ```go
  ref: models.PolicyRef{
      Source:    sourceName,
      Name:      gnp.Name,
      CreatedAt: creationTime(gnp),
      Order:     spec.Order,   // raw *float64; nil preserved via omitempty
      Tier:      tier,         // already computed above (defaults to "default")
  },
  ```
  No change to `PolicyRef` struct needed — fields already exist.

---

### `getPolicies` silently drops `ctx` — `internal/policy/calico/evaluate.go:189`
[IGNORE]
- **Severity:** High
- **Why this severity:** `getPolicies` signature is `func (s *source) getPolicies(_ context.Context)`. `ctx` is received from `Evaluate` (which gets it from `store.PopulateCache` via `context.Background()`) but is never threaded to `s.client.GetGlobalNetworkPolicies()`. The `KubernetesClient` informer lister (`gnpLister.List(labels.Everything())`) is an in-memory call that can't use a context, so the current implementation is technically safe. However: if the lister is ever replaced with a live API call (e.g., during a client refactor), or if a timeout wrapper is added to `PopulateCache`'s context, the calico engine will be the only one that ignores the deadline. This creates a class of hard-to-diagnose hangs. Both `k8spolicy` and `istio` engines pass `ctx` through to their `getPolicies` calls — this is the odd one out.
- **User impact:** No immediate production impact while the informer lister is in use. Becomes a 3am page if a context-deadline wrapper is ever added at the `store.PopulateCache` call site.
- **How it should be implemented instead:** Change the signature to use `ctx` even though the current lister doesn't need it — sets the right pattern:
  ```go
  func (s *source) getPolicies(ctx context.Context) ([]globalPolicy, error) {
      _ = ctx // informer lister is in-memory; context would apply to a live List call
      raw, err := s.client.GetGlobalNetworkPolicies()
  ```
  Or, if the interface ever gains a context-aware variant, wire it here.

---

### `GlobalPolicies` field is dead scaffolding — `internal/models/evaluation.go:14`

- **Severity:** High
- **Why this severity:** `EvaluationResult.GlobalPolicies map[string]NodeRules` was added to the struct with a detailed comment ("cluster-scoped policy name → its ingress/egress rules. Catalog of ALL globals (inactive ones keep an empty bucket)"). Nothing in the codebase writes to it — `grep -rn "GlobalPolicies"` returns only the struct definition. Nothing reads it — `buildGraph.go`, `buildStore.go`, `reachability.go`, `buildIssues.go` all ignore it. `calico.Evaluate` doesn't initialize it (no `GlobalPolicies: map[string]NodeRules{}` in the result literal at `evaluate.go:42-49`). This is a nil map in every actual result. If any future code reads `result.GlobalPolicies[key]` on a calico result without a nil check, it panics. The field's comment describes a useful feature (showing inactive policies) but the feature is completely absent — it's an undocumented design sketch in a production struct.
- **User impact:** No runtime impact now. Trap for the next developer who sees the comment and tries to use the field without checking whether calico (or any engine) actually populates it.
- **How it should be implemented instead:** Two options. (A) If the feature isn't planned for this branch, delete the field and the comment. (B) If it is planned, add a `// TODO: not yet populated` comment and add it to calico's result initializer so it's at minimum an empty non-nil map. Don't leave an undocumented nil.

---

## Medium

### No test for `checkPeerSupported` rejection path — `internal/policy/calico/decode.go:110-130`

- **Severity:** Medium
- **Why this severity:** `checkPeerSupported` is the guard that prevents label-scoped peers from silently broadening to catch-all. If it regresses (e.g., someone changes `isCatchAllSelector` to be more permissive), a rule with `destination.selector: "role == 'db'"` would render as `catch-all CIDR` and emit a false allow edge to every CIDR bucket. There is no test that constructs a `calicov3.Rule` with a non-catch-all peer selector and asserts that `toGlobalPolicy` errors. `TestDecodeAndEvaluate_EnrollScenario` exercises the happy path but not the rejection path.
- **User impact:** A regression in `checkPeerSupported` would silently widen allows to destinations a policy never intended to permit. That's the exact "quietly lies" failure mode this project must not ship.
- **How it should be implemented instead:** Add a table-driven test in `calico_test.go` that calls `toGlobalPolicy` with rules carrying: (a) `peer.Selector = "role == 'db'"`, (b) `peer.NotSelector = "env == 'prod'"`, (c) `peer.NamespaceSelector = "app == 'ns'"`, (d) `peer.NotPorts = [...]`. Assert that each returns a non-nil error. Runs against real `calicov3` structs so the test exercises the full conversion path.

---

### `store.Cache` type is dead code — `internal/store/store.go:8-11`

- **Severity:** Medium
- **Why this severity:** `store.Cache` (two fields: `EvaluationResults` and `NsIndex`) is defined in `store/store.go` and is never instantiated or referenced anywhere in the codebase outside its own file. Every caller uses `models.Cache`, which is the real cache type (includes `WorkloadByID`, `MeshMembership`, `MeshMetrics`, `MeshIssues`). The smaller `store.Cache` type appears to be a leftover from an earlier design that was superseded by `models.Cache`. It creates a naming trap: a new contributor importing `store` might use `store.Cache` thinking it's the active type.
- **User impact:** No runtime impact. Confusion risk for the next person touching the store package.
- **How it should be implemented instead:** Delete `store.Cache`. If it was intended as a narrower view for testing, replace with a comment pointing to `models.Cache` and the existing test helpers.

---

### `toCleanYAML` leaves `resourceVersion`, `uid`, `generation` on wire — `internal/api/get-manifest.go:117-120`

- **Severity:** Medium
- **Why this severity:** Four cleanup calls are commented out: `SetResourceVersion("")`, `SetUID("")`, `SetGeneration(0)`, `SetCreationTimestamp(metav1.Time{})`. Only `managedFields` and the `last-applied-configuration` annotation are actually stripped. A GlobalNetworkPolicy response will carry the server-assigned `resourceVersion`, `uid`, and `generation` fields. These are not sensitive but they are server internals — the manifest endpoint's stated goal is "output matches kubectl get -o yaml", and `kubectl get` strips these. Operators who paste this YAML into GitOps tooling will get a dirty diff.
- **User impact:** The manifest YAML for any Calico (or k8s/istio) policy includes `resourceVersion`, `uid`, `generation`. Cosmetic issue; doesn't prevent use but produces noisier GitOps diffs.
- **How it should be implemented instead:** Uncomment lines 117-119 (`SetResourceVersion`, `SetUID`, `SetGeneration`). Leave `SetCreationTimestamp` commented if showing the creation date is intentional for the UI context. This is a two-line change.

---

### Pass semantics silently wrong when tiers ship — `internal/policy/calico/buildIndex.go:103-109`

- **Severity:** Medium
- **Why this severity:** In `buildBucketCandidates`, a Pass rule causes a `break` from the inner rule loop, which ends that policy's contribution to the bucket's candidate list. The outer loop then advances to the next `globalPolicy`. This is correct today (one flat "default" tier). When real tiers are introduced: a Pass at the last rule of a policy in tier-A should fall through to tier-B's policies, not just to the next policy in the same tier. No tier-boundary check exists. The `sortByPrecedence` function does sort by tier first, so with multiple tiers the policies ARE ordered correctly; the bug is that Pass within tier-A would yield control to the next policy in tier-A rather than jumping the tier boundary. It's a silent first-match error — wrong verdict, no panic, no log.
- **User impact:** Today: no impact (single tier). When tiers ship: workloads in tier-A Pass to tier-B's default would instead be governed by tier-A's next policy, potentially wrong-answering both reach detection and issue classification.
- **How it should be implemented instead:** File a follow-up ticket now (before this branch ships to customers running multi-tier Calico). The fix when it lands: in `buildBucketCandidates`, track tier boundaries and on Pass, skip all remaining candidates from the same tier rather than just the current policy.

---

### `deriveTypes` fallback misreads a hand-crafted empty-rule policy — `internal/policy/calico/decode.go:166-188`

- **Severity:** Medium
- **Why this severity:** `deriveTypes` fallback (when `Types` is empty): `if len(egress) == 0 { doesIngress = true }`. A GNP with no `Types`, no `Egress`, no `Ingress` rules gets `doesIngress=true`. This hits `anyGoverns(selecting, ingress)=true` → `hasCatchAll(ingressEdges)=false` → `denyAllBaseline` synthesized. A stub or WIP policy with zero rules would appear to block all ingress. The apiserver always populates `Types` for properly submitted GNPs, so this only bites demo YAML or hand-crafted fixtures, but it's a silent wrong answer. The code comment on `deriveTypes` acknowledges "possible in hand-written demo YAML" but doesn't note the implied consequence.
- **User impact:** In DEMO_MODE (if a GlobalNetworkPolicy YAML fixture is added without explicit Types), the engine would assert default-deny ingress for a policy with zero rules. The graph would show a deny badge the cluster doesn't actually enforce.
- **How it should be implemented instead:** If both rule arrays are empty AND Types is unset, treat the policy as having no opinion on either direction: `doesIngress=false, doesEgress=false`. A no-rules policy is a selector with no clauses; it selects workloads but doesn't restrict them. Add a guard: `if len(ingress) == 0 && len(egress) == 0 { return }` before the current fallback logic.

---

## Low

### Commented-out dead struct in `calico.go` — `internal/policy/calico/calico.go:89-94`

- **Severity:** Low
- **Why this severity:** Lines 89-94 are a commented-out `policyMatch` struct with no content (`activePolicy` field has no type, `dstNode` is an orphaned field comment). This is leftover design scratch.
- **User impact:** None. Cosmetic noise.
- **How it should be implemented instead:** Delete lines 89-94.

---

### `normalizeOrder` ADR comment is a question, not documentation — `internal/policy/calico/utils.go:15`

- **Severity:** Low
- **Why this severity:** `// normalizeOrder — unset order sorts last. (verify semantics — ADR 0005 q#2)` — the parenthetical is a TODO left in committed code. ADR 0005 exists (`docs/arch/0005-calico-global-policy-support.md`). Either the question is answered (remove the parens) or it's still open (file a ticket, remove the comment).
- **User impact:** None. Cosmetic.
- **How it should be implemented instead:** Read ADR 0005 q#2 result, remove the `(verify…)` annotation.

---

### `buildTestPolicy` uses 5-positional-arg signature — `internal/policy/calico/calico_test.go:18`

- **Severity:** Low
- **Why this severity:** `buildTestPolicy(name string, doesEgress, doesIngress bool, egress, ingress []calicoRule)`. The egress/ingress parameter order is non-obvious (does egress come first or last?). Not a defect today, but the next test author will flip them. Existing tests all pass the right order, but `TestResolveWorkload_CoverageAllowAll` passing `nil` for the last arg could look like "ingress is nil" when it's actually correct.
- **User impact:** None. Next-test-author trap.
- **How it should be implemented instead:** Struct: `type testPolicySpec struct { name string; egress, ingress []calicoRule; doesEgress, doesIngress bool }`. One-liner constructor.

---

## Not-a-problem (called out to prevent churn)

### v1 Critical #1 — engine inert (`getPolicies` returned nil, nil)
Resolved. `getPolicies` at `evaluate.go:189` calls `s.client.GetGlobalNetworkPolicies()`. The `InformerClient` lister at `informerClient.go:237-242` and `DemoClient` at `demo.go:198-200` are both wired. `TestEvaluate_NoPoliciesYieldsEmptyResult` drives the full live path with a real `DemoClient`.

### v1 Critical #2 — `parseSelector` only handled `""` and `"all()"`
Resolved. `utils.go:50-56` now wires to `calicoselector.Parse` from the vendored parser. `TestDecodeAndEvaluate_EnrollScenario` exercises the `in {…}` namespaceSelector against the real parser and asserts enrolled-vs-non-enrolled namespace discrimination.

### v1 Critical #3 — unenforced marker landing in `.Allow` with zero-value `ActionAllow`
Resolved. `unenforcedRule` at `evaluate.go:177-183` now carries `Coverage: models.CoverageUnenforced` explicitly, and `denyAllBaseline` at `evaluate.go:158-172` goes into the `.Deny` bucket with `ActionDeny + Coverage: CoverageDenyAll`. `collectToSide` / `collectFromSide` in `reachability.go` handle `CoverageUnenforced` by dropping it (`// no opinion, drop`) and `CoverageDenyAll` by routing to `DenyAllMatches`, both correct. `TestResolveWorkload_UnenforcedEgressWhenIngressOnly` covers the unenforced path.

### v1 Critical #4 — `ctx` dropped at `getPolicies(_ context.Context)`
Partially resolved: the signature now receives `ctx` and callers pass it. The lister is in-memory so `ctx` can't be threaded further — this is the correct behavior for an informer-backed client. Still filed as High above because `_ context.Context` discards it by convention rather than passing it to a context-aware call, which is a structural pattern mismatch with the other engines.

### v1 Critical #5 — `isPass` `break` + tier semantics
Still open — filed as Medium above. No tiers in production today; silent bug only when multi-tier support ships.

### v1 finding — `EvaluationResult` init inconsistency (nil-map panic risk)
Still open for `GlobalPolicies`. The six maps calico initializes (`AllowByNs`, `DenyByNs`, `PolicyStatuses`, `NodePolicies`, `NodeRules`, `Nodes`) are all non-nil. `GlobalPolicies` is new (added by this branch) and is never initialized by any engine — filed as High above.

### `checkPeerSupported` failing loud on label-scoped peers
Working correctly. A rule with `peer.Selector = "role == 'db'"` causes `toGlobalPolicy` to return an error that propagates through `getPolicies` → `Evaluate` → caller. No silent broadening. Test coverage for the rejection path is missing — filed as Medium above.

### Vendored `calicoselector` — provenance and license
Apache-2.0, Tigera copyright. Copied from commit `3d00673793c8` (2026-07-26), import paths rewritten, Tigera copyright headers preserved in every file per §4(c). `go.mod` adds `github.com/sirupsen/logrus v1.9.4` as a real dependency (parser uses it for debug logging). README at `internal/thirdparty/calicoselector/README.md` documents origin commit, fetch date, why import is impossible, local modifications, and update procedure. No supply-chain concern.

### `dropRestatedEdges` correctness
Working correctly. The catch-all winner suppression logic at `buildRules.go:62-85` correctly identifies when a catch-all winner claimed all named-CIDR buckets and drops the restatements. `TestResolveWorkload_CatchAllWinnerDoesNotRestateNamedNets` covers both orderings (catch-all first vs named first).

### Empty-SrcID/DstID edges from blanket rules in graph output
Not a new defect introduced by calico. `k8s` engine has emitted `CoverageDenyAll` and `CoverageUnenforced` rules with one empty endpoint since before this branch. `buildIssues.go:178-183` guards against them (`srcOk`/`dstOk` checks). The graph layer emits these as edges with `Source=""` or `Target=""` — existing UI behavior, not a calico regression.

### No DEMO_MODE calico fixtures
By design for now. `DemoClient` parses `GlobalNetworkPolicy` kind from YAML (`demo.go:118-124`). Adding a fixture is a separate task; the decode path is exercised by `calico_test.go`.

### `collapseUniformRules` implementation in `renderEngine.go`
Well-implemented. The `NamespaceWide` gate (line 61) correctly limits collapse to engine-asserted intent, not observed coverage. `TestRenderEdges_CollapsesUniformNamespaceFanOut` covers the three-workload collapse case with calico source. `AggregatedFrom` is correctly stamped and propagated to the UI contract.

---

## Priority action order

1. **Populate `Tier` and `Order` in `PolicyRef`** — `decode.go:64-68`. Calico's primary differentiator from k8s NetPol is tier+order; omitting them from culprit chips is a day-one UX gap the operator will feel immediately.
2. **Delete `GlobalPolicies` or initialize it** — `models/evaluation.go:14`. Dead nil map in a production struct is a nil-panic waiting for the next contributor.
3. **Fix `deriveTypes` fallback for zero-rule policies** — `decode.go:186`. Silent wrong answer in demo fixtures; correct before publishing demo YAML.
4. **Add `checkPeerSupported` rejection tests** — `calico_test.go`. The guard exists; one table-driven test closes the regression window.
5. **Uncomment `SetResourceVersion`/`SetUID`/`SetGeneration` in `toCleanYAML`** — `get-manifest.go:117-119`. Two-line fix, immediate GitOps UX improvement.
6. **Delete `store.Cache`** — `store/store.go:8-11`. Remove the dead type before it misleads the next contributor.
7. **File tier-aware Pass ticket** — `buildIndex.go:103-109`. Not a current bug; needs a ticket before a customer runs multi-tier Calico.
8. **Thread `ctx` properly in `getPolicies`** — `evaluate.go:189`. Low urgency while informer lister is in use; matters if the client layer ever adds deadlines.
