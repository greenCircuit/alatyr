# Policy coverage classification

Status: k8s done, Istio TODO.

## What "coverage" is

A `Coverage` tag on each `models.Rule` describing the *breadth* of what the rule
grants or denies, so the detail panel can render deny-all / allow-all cards
instead of silently showing nothing. Values today (`models.Coverage`):

- `deny-all` — direction locked, no rules permit anything
- `allow-all` — an empty peer list: everything, any namespace, internet included
- `allow-all-ns` — a catch-all peer scoped to one namespace (stays in cluster)
- `restricted` — specific selector / port / CIDR
- `unenforced` — policy present but does not govern this direction (recorded for
  visibility, never a lie about enforcement)

The point is **visibility**: a policy that denies everything, or opens
everything, must surface — hiding it (emitting no rule) is worse than no graph.

## k8s (done)

`internal/policy/k8spolicy/buildRules.go`:

- `expandEgressRules` / `expandIngressRules` emit a marker when the direction has
  zero rules: `deny-all` if the direction is locked (`egressLocked` /
  `ingressLocked`), else `unenforced`.
- Empty peer list (`to:[{}]` / `from:[{}]`) → `allow-all`, carrying ports
  (`AllPorts` only when portless).
- Catch-all podSelector peer → `allow-all-ns` (set in `expandPeerRules`).
- Lock is read from `PolicyTypes`, not the rule slice. Ingress is *implied*
  locked on empty `PolicyTypes` (k8s: all policies affect Ingress); egress never.

Tests: `internal/policy/k8spolicy/coverage_test.go` (classification, ports,
markers) + updated cases in `edge_test.go`.

## Istio — why it needs its own logic, not a copy

Istio `AuthorizationPolicy` semantics differ from k8s NetworkPolicy on every
axis that matters here. Do **not** port the k8s branches verbatim.

1. **Ingress only.** AuthorizationPolicy gates traffic *into* the selected
   workload. Egress is Sidecar / ServiceEntry, out of scope. So there is **no
   egress coverage** — half the k8s cases don't exist.

2. **Explicit action, not structural deny.** `Spec.Action` is
   `ALLOW(0) / DENY(1) / AUDIT(2) / CUSTOM(3)`. Coverage must pair with the
   action — an `allow-all` under DENY and under ALLOW are opposite intents.
   AUDIT/CUSTOM don't allow or deny L3 at all.

3. **Inverted default.** k8s ingress is default-deny once *any* policy selects
   the workload. Istio is default-**allow** until the first ALLOW policy selects
   the workload, then default-deny-except-matches. So "no rules" means different
   things per action (see below). `unenforced` maps poorly — an Istio workload
   with no selecting ALLOW policy is *open*, not unenforced.

4. **Root-namespace policies.** Policies in the mesh root ns (e.g.
   `istio-system`) apply mesh-wide. Coverage attribution must name the origin ns,
   not assume the workload's ns.

5. **L7.** A rule can restrict by host/method/path with no L3 narrowing. An
   L7-only rule is not `allow-all` even if its `to`/`from` is empty.

## The gap in the current Istio rule layer

`internal/policy/istio/buildRules.go` `expandRules`:

- When `Spec.Rules` is empty, the `for ruleIndex, rule := range rules` loop never
  runs → `rulesMatrix` is empty → **nothing emitted**. An `action: ALLOW` policy
  with no rules is the canonical Istio **deny-all** (allows nothing) — currently
  invisible in the rules stream.
- `expandFromSource` only resolves `Source.Namespaces`. A rule with no `from`
  (any source = allow-all) produces `matchWorkloads == nil` → no rule, no marker.
  Allow-all is invisible too.

Note: `buildPolicyStatus.go` already short-circuits deny-all (:135) and allow-all
(:152) for the **badge** layer. This work is only the **rule/card** layer — same
two-layer split as k8s (status badge vs NodeInfo card).

## Use cases to add (Istio)

Detection, all ingress, all paired with `Spec.Action`:

| Case | AuthorizationPolicy shape | Coverage + Action |
|---|---|---|
| deny-all (ALLOW, no rules) | `action: ALLOW`, `rules: []` / omitted | `deny-all`, ALLOW — allows nothing |
| deny-all (DENY, catch-all) | `action: DENY`, `rules: [{}]` | `deny-all`, DENY |
| allow-all | `action: ALLOW`, rule with no `from` (any source) | `allow-all`, ALLOW |
| allow-all-ns | `from.source.namespaces: [ns]` (one whole ns) | `allow-all-ns` |
| restricted | specific `from`/`to`/principals | `restricted` |
| L7-only | `to.operation` with hosts/methods/paths, empty L3 | `restricted` (NOT allow-all) — `AllL7=false`, `L7Match` set |
| AUDIT / CUSTOM | `action: AUDIT` / `CUSTOM` | do not emit allow/deny coverage — mark distinctly or skip; these don't gate L3 |
| root-ns origin | policy in mesh root ns selecting mesh-wide | coverage as above, Contributor.Namespace = root ns |

Explicitly **not** applicable: any egress coverage; k8s-style `unenforced` on the
ingress default (Istio no-policy = open, not unenforced).

## Test cases to add

`internal/policy/istio/` (mirror `coverage_test.go` shape):

- ALLOW + empty rules → 1 `deny-all` ALLOW marker (the canonical case; currently
  emits 0 — this is the regression the feature fixes).
- ALLOW + rule with no `from` → `allow-all` ALLOW, ports carried, `AllPorts` when
  portless.
- DENY + catch-all rule → `deny-all` DENY.
- from a single namespace → `allow-all-ns`.
- specific principals/selector → `restricted`.
- L7-only rule (empty L3) → `restricted`, `L7Match` populated, `AllL7=false`.
- AUDIT and CUSTOM policies → no allow/deny coverage marker.
- root-ns policy → marker attributed to root ns, applied mesh-wide.

## Open decisions

- Coverage enum vs a separate `(Coverage, Action)` pair on the rule — k8s never
  needed Action on markers; Istio does. Decide before wiring the Istio side so
  the frontend card logic reads one shape for both engines.
- AUDIT/CUSTOM: distinct coverage value (`audit` / `custom`) vs omit. Omitting
  hides that the policy exists at all — against the visibility goal.
