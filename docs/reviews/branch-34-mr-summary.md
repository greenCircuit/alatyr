# Add Calico GlobalNetworkPolicy engine (third `PolicySource`)

Wires cluster-scoped Calico `GlobalNetworkPolicy` (GNP) as a new engine alongside `k8spolicy` and `istio`. Ships behind the existing `PolicySource` interface — `graph.PolicySources(client)` now registers `k8spolicy` + `istio` + `calico`, no orchestration change in `buildGraph.go`.

Design tracked in `docs/arch/0005-calico-global-policy-support.md`.

## Backend

### New engine — `internal/policy/calico/`
- `calico.go` — normalized `globalPolicy` / `calicoRule` / `bucketKey` / `bucketCandidate` types (raw CRD decoded once at fetch boundary, resolve walk never touches vendored types).
- `evaluate.go` — `source.Evaluate` fetches GNPs once (cluster-scoped, not per-ns), decodes, runs the precedence walk per `(workload, direction, bucket)`, emits `EvaluationResult{Allow, Deny, PolicyStatuses, NodePolicies, NodeRules, Nodes}`. Also emits `denyAllBaseline` (`Coverage: CoverageDenyAll`) and `unenforcedRule` (`Coverage: CoverageUnenforced`) markers so downstream consumers can distinguish "denied by explicit rule" from "denied by default" from "no opinion".
- `decode.go` — vendored-type → internal-type conversion. `checkPeerSupported` rejects label-scoped peers loud (would otherwise silently broaden to catch-all). `deriveTypes` fallback for GNPs missing `Spec.Types`.
- `buildIndex.go` — ns-level cache (`nsSelection`) with pre-resolved bucket candidates. Per-workload cost = selector match only, CIDR/port arithmetic hoisted.
- `buildRules.go` — resolved verdicts → `models.Rule`; `dropRestatedEdges` prevents phantom CIDR arrows when a catch-all winner already covered every named-CIDR bucket.
- `buildPolicyStatus.go` — per-workload `PolicyStatus` derived from the resolved walk (not raw policy scan — a `Deny` at order 1000 doesn't mean denied if order 500 already Allowed).
- `utils.go` — `parseSelector` wraps vendored `calicoselector.Parse`; `normalizeOrder` (unset order sorts last).

### Vendored Calico selector parser — `internal/thirdparty/calicoselector/`
- Copied from `github.com/projectcalico/calico@3d00673793c8` (Apache-2.0, Tigera copyright preserved per §4(c)).
- Full expression grammar: `==`, `!=`, `has()`, `in {...}`, `&&` / `||`, `all()`, negation. Hand-rolling would get set-membership subtly wrong.
- `README.md` documents origin commit, fetch date, local mods, update procedure.
- `go.mod` gains `github.com/sirupsen/logrus v1.9.4` (parser debug logging).

### Shared / integration changes
- `internal/k8s/k8s.go` + `informerClient.go` + `demo.go` — `KubernetesClient` gains `GetGlobalNetworkPolicies()` and `GetGlobalNetworkPolicyByName()`. Informer-backed on real client; fixture-backed on `DemoClient`. CRD-presence probe follows the `istioSecurityCRDPresent` discovery pattern.
- `internal/models/rule.go` — `PolicyRef` gains `Tier string` + `Order *float64` for Calico precedence display. `Coverage` constants (`CoverageDenyAll`, `CoverageUnenforced`) formalized.
- `internal/models/evaluation.go` — `GlobalPolicies map[string]NodeRules` field added (see "Known follow-ups").
- `internal/graph/renderEngine.go` — `collapseUniformRules` collapses namespace fan-out for `NamespaceWide`-asserted rules (Calico `selector: all()` → one arrow per `(ns, direction)`, not one per pod).
- `internal/store/reachability.go` + `buildIssues.go` — Calico-aware issue detection: `isInternalCidrAggregate`, `subsetAllowCidr`, `cidrNodeCoversPeer`, `policiesAllowingPeerSpecifically`. Coverage markers (`CoverageDenyAll` → `DenyAllMatches`, `CoverageUnenforced` → dropped).
- `internal/api/get-manifest.go` — `calico` case added to manifest endpoint so operators can pull GNP YAML from the UI.
- `internal/policy/istio/`, `internal/policy/k8spolicy/` — minor plumbing for the new `Coverage` field and `Tier`/`Order` on `PolicyRef`.

### Tests
- `internal/policy/calico/calico_test.go` (699 lines) — resolution walk, coverage markers, enroll scenario, vendored parser, `dropRestatedEdges` (both orderings), `NamespaceWide` collapse.
- `internal/store/buildIssues_test.go`, `reachability_test.go` — Calico-aware issue paths.
- `internal/graph/render_edges_test.go` — three-workload uniform-collapse case.
- All existing engine tests pass; `go build ./...` clean; `go test ./...` green.

## Frontend

- `ui/src/data/engines.ts` + `engineIcons.tsx` — `calico` registered as first-class engine (icon, filter chip, table rollup).
- `ui/src/components/TablesView/PoliciesTable.tsx`, `EngineRollup.tsx`, `IssueRollup.tsx`, `CulpritActions.tsx` — culprit chips render `Tier` + `Order`, so the operator can see *why* a given GNP fired at first-match resolution.
- `ui/src/components/DetailPanel/shared/groupNeighborsByPolicy.ts` + `rows.tsx` + `EdgeView.tsx` — GNP edges group + display alongside k8s / istio.
- `ui/src/store/policyTableEdges.ts` (+ test) — new derive layer for policy-table edge rollup.
- `ui/src/style/colors.css` — Calico engine token.
- Cleanup: three stale `REDESIGN*.md` drafts removed from `DetailPanel/`.

## Demo data

- `test-data/60-calico-globals.yaml` — three-policy egress guardrail scenario (enroll → HTTPS-only → cluster-only baseline) exercising precedence + `NamespaceWide` collapse.
- `test-data/40-private-net.yaml` — private-net workload for CIDR bucket coverage.

## Known follow-ups (see `docs/reviews/branch-34-calico-audit-v2.md`)

Non-blocking; filed for a follow-on PR:

- `EvaluationResult.GlobalPolicies` — field defined but not populated; either wire it or delete.
- Multi-tier `Pass` semantics — correct today (single flat tier); needs tier-boundary check before customers ship multi-tier GNPs.
- `checkPeerSupported` rejection-path test.
- `deriveTypes` fallback for zero-rule policies.
- `toCleanYAML` — uncomment `SetResourceVersion` / `SetUID` / `SetGeneration` for GitOps-clean manifest output.
- Delete dead `store.Cache` type.

## Out of scope

- Namespaced Calico `NetworkPolicy` (Calico's own CRD, distinct from k8s NetworkPolicy) — separate ADR if needed.
- Non-CIDR peers (label-scoped selectors on rule `source`/`destination`) — rejected loud by `checkPeerSupported` until wired.
- Multi-tier ordering across custom `Tier` resources — works for single "default" tier today.
