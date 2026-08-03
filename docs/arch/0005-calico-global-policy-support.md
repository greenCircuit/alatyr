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

- **Correction:** no `buildGraph.go`/per-ns fan-out change needed *for the fetch*. Engines are driven from `Builder.PopulateCache` (`/app/internal/store/buildStore.go:149`), which calls `source.Evaluate(ctx, namespaces, indexByNS)` once per engine already — `istio`'s own `Evaluate` (`/app/internal/policy/istio/evaluate.go:37-45`) already does an extra out-of-band fetch (`GetAuthorizationPolicies(RootNamespace)`) alongside its per-ns fetch, entirely inside its own `Evaluate`, with zero orchestration changes. `calico.Evaluate` fetching `GetGlobalNetworkPolicies(ctx)` once is the same shape — a one-line addition to `calico`'s own `Evaluate`, not a `buildGraph.go` change.
- **But the istio precedent only covers the fetch half, not the edge half.** Read the comment at `/app/internal/policy/istio/evaluate.go:34-36`: istio's root-ns (mesh-wide) policies feed `PolicyStatus`/badges but explicitly **do not yet drive `Rule`/edges** — cross-namespace edge fan-out is an open TODO in the existing code (`buildRules` selects workloads by the policy's own namespace). GlobalNetworkPolicy needs exactly that unsolved half: a cluster-scoped policy (own namespace `""`) rendering edges against workloads in namespaces the graph never fetched under it. So "fetch once, zero orchestration change" is confirmed; "cross-namespace edge rendering is a solved pattern to copy" is **not** — istio hasn't solved it either. The precedence-walk section below owns the rule-generation side; don't assume it falls out of the istio fetch pattern.
- `k8s.KubernetesClient` interface gets a new method (e.g. `GetGlobalNetworkPolicies(ctx) ([]*calico.GlobalNetworkPolicy, error)`), implemented on both `Client` (needs Calico client-go, e.g. `github.com/projectcalico/api/pkg/client/clientset/versioned`) and `DemoClient` (fixture-backed). CRD-presence probe pattern to copy is `istioSecurityCRDPresent` in `/app/internal/k8s/informerClient.go:113` (discovery-based), **not** `mesh/istio/detect.go` (that's ambient-membership label detection, unrelated). **(verify: confirm actual Calico Go module path and CRD version the target cluster runs — Calico has had several API package moves.)**
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

### Selector evaluation & memoization (backend-sre review, 2026-07-25)

**No intermediate working struct.** Walk the raw `*v3.GlobalNetworkPolicy` objects straight from the informer — `Spec.Order`, `Spec.Tier`, `Spec.Egress[]`/`Spec.Ingress[]` (array order intact — load-bearing, see below), `Spec.Selector`/`Spec.NamespaceSelector` are all already on the object. Copying them into a `calicoPolicy` struct is pure ceremony.

**The one thing worth memoizing is the parsed selector.** `Spec.Selector` is a *string*; `selector.Parse` on every workload×policy is the only real cost, and it's an artifact of parsing in the wrong loop. Hoist it: parse each policy's selector once into a separate `map[string]selector.Selector` keyed by policy UID, up front. Parse cost becomes O(policies) ≈ tens, one-time. What's left per node is `selector.Evaluate(labels)` against the already-parsed AST — label-map lookups, no re-parse. Tens of policies × thousands of pods = tens of thousands of cheap evals.

**Namespace-selector memo, same bucketing instinct as `resolveMtls`.** `namespaceSelector` evaluates against *namespace* labels, and every pod in a ns shares them. Memoize `nsSelector.Evaluate(nsLabels)` per `(policy, namespace)` — tens × tens — not per pod. A workload matches only if both selectors pass, so gate on the cached ns result first and skip the pod-selector eval entirely for namespaces the nsSelector already excluded. (Answers open question #4: the per-workload matching needs namespace *labels* accessible — confirm `NSIndex` carries them.)

Cost profile after hoisting: parse once per policy (negligible); nsSelector eval once per `(policy, ns)`; pod selector eval once per `(policy, workload)` only in namespaces that passed. Nothing pathological at target scale.

**Sort once, not per workload.** Sort the raw `[]*v3.GlobalNetworkPolicy` by `(tier, order asc, name)` a single time before the workload loop, not inside it.

**Resolves open question #3: use the vendored libcalico parser, don't hand-roll.** `github.com/projectcalico/calico/libcalico-go/lib/selector` — `selector.Parse(str)` → `.Evaluate(labels map[string]string) bool`. It covers the full expression grammar (`==`, `!=`, `has()`, `in {...}`, `&&`/`||`, `all()`, negation) that `map[string]string` can't hold. A hand-rolled evaluator will get set-membership or negation subtly wrong and lie. Confirm the exact module path against the vendored client version (ties into open question #1).

**Selected-but-shadowed ≠ doesn't-apply — tri-valued, don't collapse to active/inactive.** The precedence walk produces three distinct per-workload states for a global policy, and rendering them as a binary would repeat the "quietly wrong" failure this project rejects:
- **wins** — selected the workload *and* its first-match rule decided a destination bucket.
- **applies-but-shadowed** — selected the workload (both selectors pass) but every rule lost first-match to a lower-`order` policy. Example: for an enrolled-ns workload, `egress-default-deny` (order 2000, `selector: all()`) selects it but is fully shadowed by `egress-enroll-namespaces` (order 500) Allow-all.
- **doesn't-apply** — `namespaceSelector`/`selector` excluded it; never evaluated. Example: `egress-enroll-namespaces` against a non-enrolled workload.

`NodePolicies[workload]` holds only the policies that **select** the workload (states 1–2). Shadowed losers attach per resolved edge as provenance (winner + `shadowed []PolicyRef`), keyed on `(workload, dstBucket)` — not per workload. A policy that doesn't select is simply absent, not "inactive." Showing a non-selecting policy as evaluated-and-lost tells the user it was in scope when it never was.

### CIDR / nets matching (not just label selectors)

The selector section above covers *which workloads a policy governs*. It does **not** cover *what a rule's source/destination points at* — and Calico rules routinely use CIDR lists, not workload selectors, for that. A rule carries `source`/`destination` blocks with `nets []string`, `notNets []string`, `selector`, and `namespaceSelector`. An engine built only to the label-selector model would silently drop or mis-evaluate every `nets`-based rule — and the "deny internet egress" pattern (`destination: {notNets: [pod-cidr, svc-cidr]}` or the first-use-case `destination: {nets: [...]}` + `Deny {}`) is one of the most common reasons anyone writes a GlobalNetworkPolicy at all. Dropping it is the exact "graph that quietly lies" failure this project rejects. Required subsection, not a footnote.

Reuse the existing classifier — do not build a second one. `policy/networkPolicyHelpers.go` already ships engine-agnostic CIDR helpers (CLAUDE.md calls them "usable across engines"):

- `CidrType(cidr)` → `CIDRWan` / `CIDRk8sPod` / `CIDRk8sSvc` / `CIDRLan` by containment against `config.PodCIDR`/`SvcCIDR`/`ApiServerCIDRs`. Maps each `net` to the right CIDR node.
- `CidrContains(outer, inner)` → the containment test for matching a resolved destination bucket against a rule's `nets`.
- CIDR-node emission: copy the `cidrNodes[models.CIDRIDPrefix+cidr] = WorkloadNode{...}` pattern from `k8spolicy/buildRules.go:296`.

Two translation gaps the k8s helpers don't cover (Calico-engine-side, not classifier changes):

1. **`destination: {}` (empty) means *all destinations*, not internet.** The helpers take a CIDR string; empty isn't one. Normalize `{}` → catch-all bucket = `0.0.0.0/0` → `CIDRWan`. For the fallthrough `Deny {}` after an `Allow <cluster>`, the honest render is deny→`0.0.0.0/0`, since cluster was already allowed by the prior (higher-precedence) rule.
2. **`nets` is a list; `notNets` is exclusion.** k8s helpers take one `IPBlock`; call `CidrType`/`CidrContains` per entry. `notNets` inverts the match — defer to a follow-on unless the target cluster's policies use it (the first-use-case manifests don't; the common internet-deny pattern above does, so scope this deliberately).

**Config dependency, load-bearing:** the pod/svc buckets are correct only if `config.PodCIDR`/`SvcCIDR` are set to the cluster's actual ranges (k3s: `10.42.0.0/16` / `10.43.0.0/16`). Unset → `10.42/16` falls to `CIDRLan` and the whole "in-cluster vs internet egress" story collapses into a LAN blob. Verify config before trusting the buckets.

**Destination universe is bounded — no IP-space or pod-pair explosion.** Resolve per `(workload, direction, destination-bucket)` where the bucket set = every `net` referenced by any selecting policy + the `{}` catch-all. For the first-use-case manifests that's 4 buckets. Walk ordered rules per bucket, `{}`-rule matches every bucket, else `CidrContains(rule.net, bucket)`. Same first-match machinery as the label path.

### First-match is at rule-array granularity *within* a policy, not just across policies

Load-bearing and easy to get wrong: `egress-default-deny` is a single policy whose two rules — `[Allow <cluster-nets>, Deny {}]` — decide *different* destinations. `10.42.x` → rule 1 Allow wins; `8.8.8.8` → rule 1 no match, rule 2 Deny wins. The walk must be `for policy in sortByOrder: for rule in policy.Egress (source array order): if matches → decide`. If you sort or union a policy's rules and lose array index, you invert this policy. This is the concrete reason precedence stays engine-internal scratch and never lands as a `weight` field on `models.Rule` — resolution is per-rule-in-sequence, not a scalar per rule.

### Ingress and egress are independent walks

`Spec.Ingress` and `Spec.Egress` are separate rule arrays with separate `Spec.Types` gating (a policy with `types: [Egress]` contributes nothing to ingress even if it selects the workload). Run the precedence walk once per direction with its own tier/order state — do not share a resolved verdict across directions. The first-use-case policies are `types: [Egress]` only; an ingress-bearing policy is a distinct walk.

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
3. ~~Calico selector expression language: hand-roll a small evaluator, or is there a vendored parser worth reusing?~~ **Resolved:** use libcalico's `selector` package (`Parse` + `Evaluate`), memoized per policy UID — see Selector evaluation section. Don't hand-roll.
4. `namespaceSelector` on a `GlobalNetworkPolicy` (verify) — does the graph's per-namespace `NSIndex` fetch have to expand to include namespace *labels* somewhere accessible for cluster-scoped matching, or is that already available? (See Selector evaluation section — nsSelector matching needs ns labels reachable per workload.)

(Resolved during backend-sre review, no longer open: `NodePolicies` shape needs no new type — `map[string][]PolicyRef` suffices once Pass-suppressed policies get a `PolicyRef{Action: "pass"}` unconditionally, see Decision above. `AllowByNs`/`DenyByNs` keying is resolved as workload-namespace keying + a required `layering.go` fix, not left open.)

## Consequences

**Positive:**
- Reuses the proven `resolveMtls`-style precedence pattern instead of inventing a new one.
- Keeps the "edges show every contributing policy, badges show resolved intent" split consistent with the rest of the system.

**Negative:**
- Largest per-engine implementation to date — precedence walk + selector-expression evaluation are both new categories of logic this codebase hasn't needed before, and the walk runs per (workload, direction, port), not per workload.
- `PolicySelector.LabelSelector` no longer covers every source's selector shape. Confirmed additive, not breaking: `LabelSelector` is populated by `k8spolicy/buildRules.go` and `istio/buildRules.go` purely for detail-panel display (`store/utils.go:16-17`) — it's never used as matching input, matching happens against the real k8s/Istio selector types elsewhere. Fix is a new `Expression string` field alongside the map, plus a frontend code path to render an expression string instead of label chips when it's set.
- `layering.go`'s culprit-namespace join (`store/layering.go:133`) requires a required (not follow-on) fix to handle `Contributor.Namespace == ""` as the cluster-scoped case — ships in the same PR as the Calico engine, not after.
