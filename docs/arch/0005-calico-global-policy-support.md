# ADR 0005: Calico GlobalNetworkPolicy Support

**Status:** Proposed
**Date:** 2026-07-21
**Branch:** TBD (not yet created)

## Context

Two engines are wired today: `k8spolicy` (K8s NetworkPolicy) and `istio` (AuthorizationPolicy). ADR 0001 explicitly named Calico as a planned third source, and the `PolicySource` abstraction was built for exactly this addition. This ADR scopes the first Calico cut: **`GlobalNetworkPolicy` only** — cluster-scoped, tiered, ordered L3/L4 policy. Namespaced Calico `NetworkPolicy` (Calico's own CRD, distinct from K8s NetworkPolicy) is explicitly out of scope for this branch; revisit as a follow-on ADR if needed.

**Assumption flag:** this repo has no Calico client-go dependency and no CRD types checked in yet. The API shape described below (fields, defaults, tier semantics) is drawn from Calico's public API — it has not been verified against a live cluster's installed CRD version or against vendored Go types. Confirm exact field names/defaults against `projectcalico.org/v3` before implementation starts; treat anything below marked **(verify)** as a to-do, not a fact.

**Correction (backend-sre review, 2026-07-21):** orchestration has moved on since CLAUDE.md's description — engines are driven from `Builder.PopulateCache` / `/app/internal/store/buildStore.go:150`, which calls `source.Evaluate(ctx, namespaces, indexByNS)` once, sequentially, per engine. There is no per-namespace fan-out at the `buildGraph.go` layer for engines to hook into or avoid. This changes the "Fetch strategy" section below — flagged inline.

## Why GlobalNetworkPolicy is structurally different from the existing engines

- **Cluster-scoped, not namespace-scoped.** `k8spolicy` and `istio` both fetch per-namespace and evaluate inside the `namespaces` param the graph was built for. `GlobalNetworkPolicy` has no `namespace` field — it selects workloads across the whole cluster via `spec.selector` (a Calico selector expression, not a label map) plus optional `spec.namespaceSelector`. It must be fetched once per graph build, not per-ns, and then matched against every workload regardless of which namespaces the caller asked to see.
- **Selector language is not `map[string]string`.** Calico selectors are boolean expressions over labels (e.g. `role == 'db' && !has(deprecated)`). `models.PolicySelector.LabelSelector` (rule.go:51) assumes an exact-match label map, which is a K8s/Istio-shaped assumption baked into the existing type. This either needs a new selector-evaluation path or `PolicySelector` needs a variant that carries the raw expression for display without pretending it's a label map.
- **Ordered, tiered, with a `Pass` action.** This is the part with no existing precedent in the two current engines. `k8spolicy` and `istio` both render every matching rule as its own edge with no cross-rule precedence — "no reducer at the edge level" (CLAUDE.md, k8spolicy/istio design decisions). Calico is different by design: policies within a tier are evaluated in `order` sequence (lowest first **(verify default: unset order sorts last, not first)**), and a rule's `action` can be `Allow`, `Deny`, `Pass`, or `Log`. `Pass` stops evaluation in the current tier and falls through to the next tier; if no tier explicitly allows or denies, the tier's default action applies. `resolveMtls` (`peerauth.go:27`) is the closest existing "walk + surface footguns" precedent in this codebase, but only for that instinct — see the correction under Decision below on where the structural analogy actually breaks down.

## Decision

Add a third `PolicySource`: `calico`, package `internal/policy/calico/`, mirroring the `istio/` package layout (`evaluate.go`, `buildRules.go`, `buildPolicyStatus.go`, `policyStatus.go`, `utils.go`).

**Correction: this is not the same shape as `resolveMtls`, only the same instinct.** `resolveMtls` is a fixed 3-bucket design (workload/ns/mesh), each bucket walked independently, first-non-UNSET-wins, resolving one scalar (mtls mode) with no directionality. Calico's tier count is unbounded and order is continuous — there's no fixed bucket count to force it into, and trying would be actively wrong. The right shape is a **flat sorted list + single walk** (sort by `(tier, order, name)`, walk once, first terminal Allow/Deny wins). Two things this walk needs that `resolveMtls` has no precedent for and that add real cost: (1) it must run independently per **(workload, direction, port)** — ingress and egress are separate rule sets with separate tier/order state, whereas PeerAuthentication has no directionality at all; (2) the `Issues`-style footgun surfacing (same-tier ties, dead-end Pass) is new logic specific to Calico's semantics, it doesn't fall out of reusing `resolveMtls`'s code.

### Fetch strategy

- **Correction:** no `buildGraph.go`/per-ns fan-out change needed. Engines are driven from `Builder.PopulateCache` (`/app/internal/store/buildStore.go:150`), which calls `source.Evaluate(ctx, namespaces, indexByNS)` once per engine already — `istio`'s own `Evaluate` (`/app/internal/policy/istio/evaluate.go:37-45`) already does an extra out-of-band fetch (`GetAuthorizationPolicies(RootNamespace)`) alongside its per-ns fetch, entirely inside its own `Evaluate`, with zero orchestration changes. `calico.Evaluate` fetching `GetGlobalNetworkPolicies(ctx)` once is the same shape — a one-line addition to `calico`'s own `Evaluate`, not a `buildGraph.go` change.
- `k8s.KubernetesClient` interface gets a new method (e.g. `GetGlobalNetworkPolicies(ctx) ([]*calico.GlobalNetworkPolicy, error)`), implemented on both `Client` (needs Calico client-go, e.g. `github.com/projectcalico/api/pkg/client/clientset/versioned`) and `DemoClient` (fixture-backed, same pattern as `detect.go`'s mesh-CRD-installed check). **(verify: confirm actual Calico Go module path and CRD version the target cluster runs — Calico has had several API package moves.)**
- Error handling needs no new design: `buildStore.go:151-158` already aborts `PopulateCache` and surfaces the wrapped error on any engine failure, matching `PolicySource.Evaluate`'s documented contract (`policy.go:16-17`). A Calico fetch error fails the whole graph like the other two engines — confirmed, not an open question.

### Namespace-key collision in `EvaluationResult` — real, not hypothetical

`AllowByNs`/`DenyByNs` (`models/evaluation.go:7-8`) are already keyed two incompatible ways by existing consumers, and Calico exposes the conflict rather than creating it:

- `/app/internal/store/layering.go:133` — `eval.AllowByNs[culprit.Namespace]`, where `culprit.Namespace` is the **policy's own namespace** (joined against `rule.Contributor.Namespace`). Keyed by *policy* namespace.
- `/app/internal/store/node-info.go:13` — `engineEvaluate.AllowByNs[ns]`, where `ns` is the **queried workload's** namespace. Keyed by *workload* namespace.

For `k8spolicy`/`istio` these never diverge — a namespaced policy's namespace and the namespace it was fetched under are the same string. `GlobalNetworkPolicy` has neither: `PolicyRef.Namespace` is `""` for a cluster-scoped policy, and a single policy can match workloads across every namespace. There is no single keying choice that satisfies both call sites: bucket under `""` and `node-info.go`'s per-workload-ns lookup finds nothing (Calico rules silently vanish from `GetNodeData` for every workload); bucket under each matched workload's namespace and `layering.go`'s culprit join breaks (`culprit.Namespace == ""` won't match a populated key).

**Required, not optional:** bucket Calico rules under each matched workload's namespace (satisfies `node-info.go` with zero changes there), and fix `layering.go`'s culprit join to match by `Contributor.Source + Contributor.Name` alone when `Contributor.Namespace == ""` — treat empty namespace as the cluster-scoped marker. This has to land as part of the Calico PR itself; it isn't a follow-on.

### Precedence model

- Sort policies by `(tier, order, name)` — tier order first (tiers themselves have an `order`; default tier is implicitly last **(verify)**), then policy order within tier, then name for determinism (same tie-break style as `sortByCreation` in peerauth.go:127, but by `order` instead of `creationTimestamp` since Calico's own semantics are order-based, not creation-based).
- Walk sorted policies per **(workload, direction, port)**; each policy's first matching rule (by selector + protocol/port) yields `Allow`, `Deny`, or `Pass`. `Allow`/`Deny` terminate the walk for that dimension and become the effective decision. `Pass` continues to the next tier boundary, not the next policy in the same tier **(verify: within-tier Pass behavior vs cross-tier)**.
- Every policy that matched along the way — including ones a later `Pass` overrode — still renders as its own edge/rule (consistent with "no reducer at the edge level"). The precedence walk's output is not "collapse to one edge"; it's the **effective status** fed into `PolicyStatuses`, same division of labor as today: edges show every contributing policy, `PolicyStatus`/badges show the resolved intent.
- `RuleAction` (models/rule.go:20) does **not** need a third value. Pass rules emit no `Rule`/edge at all (they have no allow/deny semantic to draw), but — hardened from an open question into a requirement below — every Pass-resolved policy still surfaces via `PolicyRef{Action: "pass"}` in `NodePolicies`. An edge saying "action: pass" would tell the user nothing actionable; a *missing* `PolicyRef` for a policy that scoped the workload would be a silent gap, which is the failure this project explicitly treats as worse than no graph at all.

### Status keys

- Per the "badges show intent, not effective behavior" rule (project memory), a naive per-rule scan (today's `generatePolicyStatusAssignment` pattern in both existing engines) is wrong here — a `Deny` rule at order 100 doesn't mean "denied" if a `Pass` at order 50 already routed elsewhere, and a policy that never matches the workload contributes nothing. `calico`'s `buildPolicyStatus` has to run the precedence walk first, then derive `PolicyStatus` from the *walk's resolved outcome* per dimension (ingress/egress × allow/deny), not from raw policy scan like the other two engines can get away with (they have no cross-policy precedence to resolve).
- This is a materially bigger lift than `istio`'s `buildPolicyStatus.go` accumulator-subtraction approach, because Calico's tiers mean the "which policies are even in scope for this workload" set depends on order, not just selector match.

### Detail panel / PolicyRef

- `PolicyRef.Action` (models/rule.go:44) gains `"pass"` alongside `"allow"`/`"deny"`. **This is not limited to anomaly cases** — every Pass-resolved policy, including the common, correctly-configured "security tier passes to app tier" case, gets a `PolicyRef{Action: "pass"}` entry in `NodePolicies`, unconditionally. Without this, a workload governed by 4 tiers that all Pass before tier 5 denies would show exactly one edge (the deny) with zero indication 4 other policies touched it — a bigger version of the same failure mode as a lying badge, except here it's the graph itself omitting policies that unambiguously scoped the workload. `PolicyRef.Action` is already a plain string, so this costs nothing in the type system.
- Consider a Calico-specific issue surface analogous to `MtlsState.Issues` (mesh.go:26) for precedence footguns worth flagging: e.g. two policies at the same `(tier, order)` (Calico allows ties; resolution order in that case is **(verify — likely name-based or undefined)**), or a `Pass` in the last tier with no fallback default, which silently allows/denies depending on tier default action.

## Alternatives considered

**A. Flatten precedence away — render every matching rule as an edge, let intersection layer sort it out like the other two engines.**
Rejected: this would misrepresent the graph. A `Deny` at order 500 that a `Pass` at order 10 already routed around isn't a real deny for that workload; showing it as one lies to the user exactly the kind of "quietly wrong" badge this project has explicitly rejected before (project memory: badges show intent, not effective behavior).

**B. Model tiers/order but skip `Pass` — treat every rule as terminal Allow/Deny.**
Smaller lift, but `Pass` is a first-class, commonly-used Calico feature (it's the mechanism for "security team's tier evaluates first, app team's tier evaluates second"). Silently mistranslating Pass as Deny or Allow is worse than not supporting Calico at all for any cluster that uses tiers this way.

**C. Full precedence + Pass walk, modeled on `resolveMtls` (chosen).**
Correct, and reuses a resolution pattern already proven in this codebase rather than inventing one. Costs the most engineering time of the three, concentrated in `buildPolicyStatus.go` and a new selector-expression evaluator.

## Open questions to resolve before implementation

1. Exact Calico Go module/CRD version to vendor against (project's target cluster version).
2. Tier `order` default and cross-tier Pass fallthrough semantics — confirm against Calico docs/source, not assumption.
3. Calico selector expression language: hand-roll a small evaluator, or is there a vendored parser in the client-go module worth reusing?
4. `namespaceSelector` on a `GlobalNetworkPolicy` (verify) — does the graph's per-namespace `NSIndex` fetch have to expand to include namespace *labels* somewhere accessible for cluster-scoped matching, or is that already available?

(Resolved during backend-sre review, no longer open: `NodePolicies` shape needs no new type — `map[string][]PolicyRef` suffices once Pass-suppressed policies get a `PolicyRef{Action: "pass"}` unconditionally, see Decision above. `AllowByNs`/`DenyByNs` keying is resolved as workload-namespace keying + a required `layering.go` fix, not left open.)

## Consequences

**Positive:**
- Reuses the proven `resolveMtls`-style precedence pattern instead of inventing a new one.
- Keeps the "edges show every contributing policy, badges show resolved intent" split consistent with the rest of the system.

**Negative:**
- Largest per-engine implementation to date — precedence walk + selector-expression evaluation are both new categories of logic this codebase hasn't needed before, and the walk runs per (workload, direction, port), not per workload.
- `PolicySelector.LabelSelector` no longer covers every source's selector shape. Confirmed additive, not breaking: `LabelSelector` is populated by `k8spolicy/buildRules.go` and `istio/buildRules.go` purely for detail-panel display (`store/utils.go:16-17`) — it's never used as matching input, matching happens against the real k8s/Istio selector types elsewhere. Fix is a new `Expression string` field alongside the map, plus a frontend code path to render an expression string instead of label chips when it's set.
- `layering.go`'s culprit-namespace join (`store/layering.go:133`) requires a required (not follow-on) fix to handle `Contributor.Namespace == ""` as the cluster-scoped case — ships in the same PR as the Calico engine, not after.
