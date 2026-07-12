# Story: MissingDns false-positives on global egress-to-any

## Status
Open — fix later.

## Summary
`MissingDns` (`internal/store/buildIssues.go:39`) flags `NoDNSEgress` issues for
workloads that actually permit DNS via an allow-all egress rule. Reachability to
the DNS peer is matched strictly by DstID, and the allow-all coverage case is not
recognized as permitting.

## Root cause
`collectToSide` (`internal/store/reachability.go:12`) sorts egress allows by:

```go
case rule.DstID == dstNodeId || rule.DstID == dstNsNodeId:
    verdict.AllowMatches = append(verdict.AllowMatches, rule)
case rule.Coverage == models.CoverageDenyAll:
    ...
case rule.Coverage == models.CoverageUnenforced:
    ...
default:
    verdict.OtherAllowMatches = append(verdict.OtherAllowMatches, rule)
```

`CoverageAllowAll` is not handled → hits `default` → `OtherAllowMatches` →
`decideDirectionVerdict` returns `ReasonLockedNoMatch` → flagged as DNS blocked.

## Breaking configs
1. `egress: - to: [] ports: [{port: 53}]` (allow-all destinations).
   `buildRules.go:132-141` emits one rule, `Coverage: CoverageAllowAll`, **DstID
   empty**. Empty never equals dnsNodeID → false `NoDNSEgress`. Most common way
   people actually open DNS.
2. `to: - ipBlock: {cidr: 0.0.0.0/0}` on port 53 (`buildRules.go:271-272`).
   DstID = CIDR string, never equals dnsNodeID → same false positive. DNS is
   reached by ClusterIP, so a to-CIDR egress is a legit DNS grant the check does
   not credit.

## What already works
- Explicit pod/ns selector resolving to the DNS pod ID or DNS ns node ID.
- Pod-catch-all-to-DNS-ns (`CoverageAllowAllNs`, DstID = that ns's NSNode.ID —
  matches dnsNsID).

## Proposed fix
In `collectToSide`'s switch, before `default`:

```go
case rule.Coverage == models.CoverageAllowAll:
    verdict.AllowMatches = append(verdict.AllowMatches, rule)
```

Egress allow-all permits every peer including DNS — belongs in `AllowMatches`.

`collectFromSide` (`reachability.go:44`) has the identical `CoverageAllowAll` gap
on the ingress side; fix there too if DNS reachability is ever evaluated in that
direction.

## Out of scope
CIDR-based DNS grants (to-`0.0.0.0/0` or the DNS ClusterIP's CIDR) still won't be
credited — that needs peer-IP-vs-service-IP resolution not present in the node
index today. Separate policy call.
