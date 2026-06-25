# Rule / NodeRule / PolicyEdge: port + L7 shape

## Problem

The current data model independently flattens ports and L7 predicates when
aggregating rules into a `PolicyEdge` (graph view) or a `NodeRule`
(detail-panel view). This produces two silent correctness bugs and one
visible cosmetic mess. None of them surface as errors — they just render
incorrect permissions to the operator, which is the exact failure mode this
tool exists to prevent.

## Current shape

```go
type Rule struct {
    SrcID, DstID string
    Direction    Direction
    Action       RuleAction
    Contributor  PolicyRef

    Port    []Port
    L7Match *L7Match
}

type PolicyEdge struct {
    // ... key fields ...
    Ports     []Port
    L7Matches []L7Match
}
```

`RenderEdges` (`internal/graph/renderEngine.go:62-64`):

```go
if rule.L7Match != nil {
    edge.L7Matches = appendUniqueL7(edge.L7Matches, *rule.L7Match)
}
if len(rule.Port) != 0 {
    edge.Ports = appendUniquePort(edge.Ports, rule.Port)
}
```

Each Rule contributes its ports to one slice and its L7 predicates to
another, deduped independently. The pairing between a rule's ports and its
L7 predicates is discarded at the edge level.

## Bug 1: L7 unrestricted silently dropped

Source spec:

```yaml
# Policy A
rules:
- to:
  - operation:
      ports: ["80"]
      hosts: [api.example.com]
# Policy B
rules:
- to:
  - operation:
      ports: ["80"]
      # no hosts/methods/paths — any L7 allowed
```

Emitted rules:

- Rule A: `Port=[80]`, `L7Match={hosts:[api]}`
- Rule B: `Port=[80]`, `L7Match=nil`

After `RenderEdges`:

- `edge.Ports = [80]`
- `edge.L7Matches = [{hosts:[api]}]`

Frontend reads "edge allows port 80 to host `api.example.com` only."
Actual permission is "port 80 to any host" (because Policy B has no L7
restriction). The unrestricted side is silently dropped because
`rule.L7Match != nil` skips Rule B.

This under-reports permissions. Operator sees fewer allowed paths than
actually exist.

## Bug 2: Port "all" silently dropped (Istio side)

Same shape, port axis. Istio policy with no `ports:` field in the
`Operation` emits `Rule.Port = nil`. `RenderEdges` skips the append.
Result: edge shows only the other rule's explicit ports, the unrestricted
rule's ports vanish.

K8s side has the inverse problem: `buildRules.go:142-144` substitutes a
sentinel `[{Port:0, Protocol:"TCP"}]` for unrestricted, which then renders
as port-0 chips alongside real ports. Visible garbage rather than silent
loss, but still wrong.

## Bug 3: Cross-rule pairing loss (matrix lie)

Two rules in the same edge bucket with different (ports, L7) pairings:

- Rule A: `Port=[80]`, `L7Match={hosts:[api]}` — port 80 to api only
- Rule B: `Port=[443]`, `L7Match={methods:[POST]}` — port 443, POST only

After independent flatten:

- `edge.Ports = [80, 443]`
- `edge.L7Matches = [{hosts:api}, {methods:POST}]`

Frontend interpretation: any port in `Ports` is allowed with any L7 in
`L7Matches`. So "port 80 to api" and "port 443 with POST" become "80 or 443,
to api, with POST" — a cartesian product that grants traffic neither policy
intended.

This over-reports permissions. Operator sees combinations that don't exist.

## Root cause

`PolicyEdge.Ports []Port` + `PolicyEdge.L7Matches []L7Match` cannot encode
"these N (ports, L7) blocks each apply independently." It can only encode
"these ports AND these L7 predicates" — implicit cartesian.

Fix requires keeping each source-spec block atomic through aggregation.

## Options

Two viable shapes. Both eliminate the bugs. Pick based on naming clarity
and migration diff size.

### Option A: Wrapper struct (`PolicyAllowance`)

Introduce a new type holding both axes plus their "unrestricted" flags.
Each Rule embeds one. Aggregation layers carry a slice.

```go
type PolicyAllowance struct {
    Ports    []Port
    AllPorts bool        // true → Ports must be empty; "no port restriction"
    L7Match  *L7Match    // nil when AllL7
    AllL7    bool        // true → L7Match must be nil; "no L7 restriction"
}

type Rule struct {
    SrcID, DstID string
    Direction    Direction
    Action       RuleAction
    Contributor  PolicyRef
    PolicyAllowance      // embedded; rule.Ports, rule.AllL7, etc. via promotion
}

type NodeRule struct {
    DstID       string
    Direction   Direction
    Action      RuleAction
    Contributor PolicyRef
    Allowances  []PolicyAllowance
}

type PolicyEdge struct {
    // ... key fields ...
    Allowances []PolicyAllowance
}
```

**Invariants** (enforced at engine emit-time, fail loud on violation):
- `AllPorts == true` ⇒ `len(Ports) == 0`
- `AllL7   == true` ⇒ `L7Match == nil`

**Aggregation:**
```go
if !containsAllowance(edge.Allowances, rule.PolicyAllowance) {
    edge.Allowances = append(edge.Allowances, rule.PolicyAllowance)
}
```
Full struct equality. Two rules collapse only when their entire allowance
is byte-identical.

### Option B: Extend `L7Match` with ports

Reuse the existing type. Add `Ports`, `AllPorts`, `AllL7` fields. Drop the
separate `Rule.Port` field.

```go
type L7Match struct {
    Ports      []Port
    AllPorts   bool
    AllL7      bool          // true → all six L7 slices empty by contract
    Hosts      []string
    Methods    []string
    Paths      []string
    NotHosts   []string
    NotMethods []string
    NotPaths   []string
}

type Rule struct {
    SrcID, DstID string
    Direction    Direction
    Action       RuleAction
    Contributor  PolicyRef
    L7Match      L7Match      // singular; one source-spec block per Rule
}

type NodeRule struct {
    // ...
    L7Matches []L7Match
}

type PolicyEdge struct {
    // ...
    L7Matches []L7Match
}
```

K8s rules carry an `L7Match` with `Ports` set and `AllL7=true` (L7 fields
all empty). Istio rules carry whatever the source spec defined.

## Comparison

| Concern | Option A: Wrapper | Option B: Extended L7Match |
|---|---|---|
| Pairing fidelity | Preserved | Preserved |
| Type naming honesty | `PolicyAllowance` reads true | `L7Match` lies (now holds ports too) |
| Migration diff | Larger (new type, replace fields) | Smaller (extend struct, rename one field) |
| Emit-site readability | `rule.PolicyAllowance = {...}` | `rule.L7Match = L7Match{...}` |
| K8s emit shape | `{Ports, AllL7:true}` | `{Ports, AllL7:true}` (same fields, weirder type name) |
| Frontend JSON field | `allowances` | `l7Matches` (misleading) |
| Cognitive load for new reader | "What's PolicyAllowance?" — answered by type doc | "Why does L7Match have ports?" — needs explanation every time |

The functional behavior is identical. Option A pays a one-time cost (new
type, larger migration) for ongoing naming clarity. Option B pays a smaller
one-time cost for ongoing naming confusion.

## Recommendation

**Option A.** Reasons:

1. The naming lie in Option B compounds. Every reader of
   `rule.L7Match.Ports` has to mentally re-parse "wait, why does an L7
   match have ports?" That cost recurs forever; the type rename is paid
   once.
2. Future engines (gateway-api, Cilium L7, OPA) may have predicates that
   aren't L7 at all but still need to ship paired with ports. A name like
   `PolicyAllowance` accommodates them; `L7Match` does not.
3. Embedding `PolicyAllowance` into `Rule` gives field-promotion ergonomics
   (`rule.Ports`, `rule.AllL7`) so emit sites read like today.

If migration size is a hard constraint, Option B is acceptable. It fixes
the bugs equally well. The cost is purely readability.

## What stays out of scope

These are independent bugs surfaced during this design discussion. Fix
alongside or separately:

- `appendUniquePort` keys dedup on port number alone
  (`renderEngine.go:82`). Two ports differing only on protocol or name
  collapse silently. Should key on `(Port, EndPort, Name, Protocol)`.
- `l7Equal` (`renderEngine.go:108`) compares string slices order-sensitively.
  Policy A `methods:[GET,POST]` and Policy B `methods:[POST,GET]` render as
  two distinct L7 blocks. Should sort on emit or set-compare.
- `Rule.Contributor` field is singular Go but JSON-tagged `contributors`
  (plural) at `rule.go:10`; same on `NodeRule.Contributor`. Frontend will
  trip when wired up.
- `internal/mesh/istio/detect.go:92` has a `len(rule.Port) == 0` branch
  inside `for _, port := range rule.Port` — unreachable today. After the
  shape change, that "unrestricted ⇒ ztunnel implicitly covered" semantic
  should live as an early `continue` keyed on `AllPorts`, before the port
  loop.

## Block-all isolation does not interact

A k8s policy with `policyTypes: [Ingress]` and empty `ingress:` array
isolates the workload — emits **zero rules**, sets
`PolicyStatus.IsolatedIngress=true`, surfaces as a node status badge in
the UI. There is no Rule, NodeRule, or PolicyEdge to model for this case.
The proposed shape change has no effect on isolation.

## Migration plan

1. Introduce `PolicyAllowance` in `internal/models/`. Add `AllPorts`,
   `AllL7` fields. Add `Validate()` enforcing the invariants.
2. Embed `PolicyAllowance` in `Rule`. Drop `Rule.Port`, `Rule.L7Match`.
3. Update engine emitters:
   - `internal/policy/k8spolicy/buildRules.go`: drop the port-0 sentinel
     (line 142-144), emit empty `Ports` with `AllPorts=true` for
     unrestricted blocks.
   - `internal/policy/istio/buildRules.go`: emit empty `Ports` /
     `AllPorts=true` consistently. Set `AllL7=true` when the Operation has
     no L7 predicates.
4. Change `NodeRule.Allowances []PolicyAllowance` and
   `PolicyEdge.Allowances []PolicyAllowance`.
5. Rewrite `RenderEdges` to append distinct allowances rather than flatten.
   Drop `appendUniquePort` and `appendUniqueL7` in favor of a single
   `containsAllowance`.
6. Update `mergeNodeRules` (per the sibling `merge-node-rule-ports.md`
   backlog) to merge by allowance equality.
7. Update `internal/mesh/istio/detect.go` `ValidateExternalRules` to read
   `rule.AllPorts` and `rule.Ports` instead of the flat slice. Early-skip
   the ztunnel check when `AllPorts=true`.
8. Frontend (`ui/src/data/policies.ts`, detail panel, filter logic):
   - Decode `allowances: PolicyAllowance[]` on `PolicyEdge` and `NodeRule`.
   - Render `AllPorts` / `AllL7` as explicit "ALL" chips, not absence of
     chips.
   - Filter predicates: `edge.allowances.some(a => a.allPorts || a.ports.includes(target))`.
9. Update tests:
   - `render_edges_test.go`: rewrite ports/L7 assertions in terms of
     allowances. Add a regression case for the pairing lie (two rules,
     different L7s, different ports → assert two allowances, not flattened).
   - `internal/policy/k8spolicy/edge_test.go`, `internal/policy/istio/istio_test.go`:
     update emit shape.
   - Add a backend test for the "rule allows all ports, another rule has
     port 80 → edge has two allowances, one with `AllPorts=true`" case.
10. e2e (`tests/k8s/`, `tests/istio/`): add at least one scenario where
    one policy is unrestricted and another is port-specific, asserting the
    edge ships both allowances distinctly.

## Why backend, not frontend

The frontend currently has no way to recover pairing — the data lost in
`RenderEdges` cannot be reconstructed downstream. Fix has to happen at
the layer that holds the original Rule list. Frontend changes are
mechanical render updates only.

Same argument applies for any future client (CLI, export, audit
pipeline): they all consume the same JSON, and they all need the same
pairing information. Centralizing the fix in the graph layer keeps every
client honest.
