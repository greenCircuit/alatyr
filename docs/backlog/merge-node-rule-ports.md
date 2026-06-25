# Merge multi-port NodeRules on the backend

## Problem

Backend emits one `models.NodeRule` per port. A NetworkPolicy listing 2 ports
becomes 2 NodeRules sharing every field except `Port`. The detail panel renders
one row per NodeRule, so the user sees duplicated entries (same workload, same
direction, same policy) that differ only by port chip.

`graph.renderEdges` already merges ports for `PolicyEdge` using the key
`(srcID, dstID, direction, policy, source, action)`. The rules path
(`/api/node-info`, `/api/reachable`) skips that step. Asymmetric — and the
per-port granularity leaks to every client.

## Files to change

### `internal/models/nodeInfo.go`

- `Port  Port  \`json:"port"\`` → `Ports []Port \`json:"ports"\``
- Doc comment: note that `NodeRule` is the rule-view type; one entry per
  logical (dst, direction, policy, action, L7) tuple, ports aggregated.

### `internal/store/buildStore.go`

Today: `toNodeRule(rule Rule, idIndex)` returns one NodeRule per Rule.
Call sites (lines 219, 224, 275, 283, 291, 299) append per-rule into:
- `GetNodeData` → `policyRules` (single bucket per engine)
- reachability assembly → `ev.Egress.AllowMatches`, `ev.Egress.DenyMatches`,
  `ev.Ingress.AllowMatches`, `ev.Ingress.DenyMatches` (4 buckets)

Replace with a merger:

```go
// mergeNodeRules collapses Rules sharing (DstID, Direction, Action,
// Contributors, L7Match) into one NodeRule with aggregated Ports.
func mergeNodeRules(rules []models.Rule, idIndex map[string]models.WorkloadNode) []models.NodeRule
```

Key shape (mirrors `renderEdges`):
- `DstID`
- `Direction`
- `Action`
- `L7Match` (pointer compare won't work — serialize or hash field-by-field)
- `Contributors` signature (sort + join `source|namespace|name|ruleIndex`)

Port dedup inside the bucket on `(Port, EndPort, Name, Protocol)`.

Call-site rewrite: collect the matching `[]models.Rule` for each bucket first
(filter by `SrcID == nodeId`, by `Direction`, by `Action`), then one
`mergeNodeRules(filteredRules, idIndex)` per bucket. Drop the per-rule
`toNodeRule` calls.

### `internal/mesh/istio/detect.go`

`ValidateExternalRules` currently reads `rule.Port.Port`. After the change,
each NodeRule carries `Ports []Port`. Adjust:

- "rule has all-ports / ztunnel coverage" check: `any(p.Port == 0 || p.Port == ZtunnelHBONEPort) for p in rule.Ports`
- "rule restricts to non-HBONE ports" check: every port in `rule.Ports`
  is non-zero and non-HBONE.

Coordinate with the direction-filtering fix already in flight — both touch
the emission loop.

### `internal/mesh/istio/evaluate_test.go`

- `allowRule` helper (line 14): build `Ports: []models.Port{{Port: port, Protocol: "TCP"}}`
  instead of `Port: ...`.
- Existing assertions stay; behavior unchanged because each fixture call
  still produces one port per NodeRule.

### `ui/src/data/policies.ts`

- `NodeRule.port: Port` → `NodeRule.ports: Port[]`

### `ui/src/components/DetailPanel/parts/policy.tsx`

- `RuleRow`: `realPorts([rule.port])` → `realPorts(rule.ports)`.
- No other changes — the previous frontend merge helper has been removed.

## Tests to add

In `internal/store/` (new file or extend existing):

1. Two Rules same `(DstID, Direction, Action, Contributors, L7Match)`,
   different `Port` → one merged NodeRule with two ports.
2. Different `Contributors` (different policy names) → two NodeRules,
   no merge across policies.
3. Different `L7Match` → two NodeRules.
4. Different `Direction` → two NodeRules.
5. Same port appearing twice across rules → deduped, one entry.

## Why backend, not frontend

- `renderEdges` already does the equivalent merge for `PolicyEdge`.
  Asymmetric otherwise.
- Frontend merge would need to run in two places (reachability +
  node-info). Future clients (CLI, export) repeat the logic.
- `NodeRule` is a view type already (resolves `DstLabel`, `DstNamespace`);
  port aggregation fits its role.
