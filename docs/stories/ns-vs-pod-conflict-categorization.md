# Story: cross-engine ns-vs-pod granularity → phantom policy conflicts

## Status
Open — design agreed, backend to implement.

## Summary
`PolicyIssues` (`internal/store/buildIssues.go:87`) flags a "policy conflict"
when one engine's ALLOW rule has a src→dst path that `IsNodesReachable`
(`internal/store/reachability.go:128`) reports denied by another engine. Engines
seed rules at different node granularity, so a coarse (namespace) allow probed
against a fine (pod) deny produces a **false conflict**.

- Istio grants at namespace granularity — peer is the ns-node id `ns/<name>`
  (`internal/policy/istio/buildRules.go:307` `expandFromSource`).
- k8s resolves a ns+pod selector to specific **pod** node ids `<ns>/<pod>`
  (`internal/policy/k8spolicy/buildRules.go:248-267`).

Coarse query IDs are ns-nodes; the collectors match exact id
(`reachability.go:51,62`: `rule.SrcID == srcNodeId || rule.SrcID == srcNsNodeId`).
k8s's admitting rule is keyed by a specific pod id, never equal to the ns-node id
→ k8s reports `locked-no-match` → coarse deny → phantom conflict falsely blaming
e.g. `services-network-policies`, when k8s actually admits the pod inside that ns.

## Fix: post-filter on coarse deny (two-sided member sweep)

Do not change `IsNodesReachable` — shared with the manual reachability UI click
path. Fix lives in `PolicyIssues`.

Run the coarse ns-level pass unchanged. Only when it returns `deny`, expand each
ns-node endpoint to its member workloads and re-probe the member cross-product.

### The two-sided trap (why single-side re-query is wrong)

The collectors match each side against **both** the pod bucket and the ns bucket
(`reachability.go:148-149` pass `srcNsId` + `srcNodeId`; collectors OR them).
Expanding one endpoint to a pod lets that side's k8s pod rule match — but the
other endpoint is still a ns-node. If k8s admits the source by `podSelector`
(not `namespaceSelector`), the source pod ids never equal `ns/<name>` and the
phantom **reappears on the source side**. So for an Istio ns→ns allow (both
endpoints ns-nodes — the case that motivates this), expand **both** sides and
sweep `srcMembers × dstMembers`.

The ns bucket keeps participating in the sweep, which is wanted: an Istio ns→ns
allow legitimately admits every member, and the collector still matches the ns id
alongside the member's pod id. The ns bucket is the friend on the allow side and
the enemy on the coarse-only pass; expanding both sides is what disambiguates.

Cost: on a ns→ns deny the re-probe is itself M×N — but paid only on already-
flagged edges, not on every ns-scoped edge. Flagged edges are rare; that's the
whole reason to post-filter instead of pre-expanding.

## Draft (suggestion — backend implements)

Endpoint expansion. Pod id resolves to itself; ns-node fans out to members.
`verified=false` means a ns-node we could not resolve (empty ns / NsIndex miss) —
caller must not treat the sweep as authoritative.

```go
// expandEndpoint returns the concrete workload ids to probe for a rule endpoint.
// A pod id resolves to itself; a ns-node id fans out to the namespace's member
// workloads (excluding the namespace node). verified=false → ns-node with no
// resolvable members; caller must not trust a sweep built on it.
func expandEndpoint(data *models.Cache, id, namespace string) (ids []string, verified bool) {
	idx, ok := data.NsIndex[namespace]
	// Pod endpoint (or unknown ns): probe the id itself.
	if !ok || idx.NSNode == nil || id != idx.NSNode.ID {
		return []string{id}, true
	}
	// ns-node endpoint: fan out to member workloads.
	for i := range idx.Workloads {
		workload := &idx.Workloads[i]
		if workload.Type == models.NodeTypeNamespace {
			continue
		}
		ids = append(ids, workload.ID)
	}
	if len(ids) == 0 {
		return []string{id}, false // ns-node but no members — unverifiable
	}
	return ids, true
}
```

Member sweep + tally. One `IsNodesReachable` per member pair; record the blocked
pairs with their blocking engines for culprit attribution.

```go
type memberBlock struct {
	src, dst string
	engines  map[string]EngineVerdict // blocking engines for this pair
}

type sweepTally struct {
	total   int
	blocked int
	blocks  []memberBlock
}

// sweepMembers probes every src×dst member pair and tallies genuine blocks.
func sweepMembers(ctx context.Context, data *models.Cache, noMesh []mesh.MeshSource, srcIds, dstIds []string) sweepTally {
	var tally sweepTally
	for _, srcId := range srcIds {
		srcNode, err := IdtoWorkload(data, srcId)
		if err != nil {
			continue
		}
		for _, dstId := range dstIds {
			dstNode, err := IdtoWorkload(data, dstId)
			if err != nil {
				continue
			}
			tally.total++
			result := IsNodesReachable(ctx, data, noMesh, srcId, srcNode.Namespace, dstId, dstNode.Namespace)
			if result.Verdict != "deny" {
				continue
			}
			tally.blocked++
			tally.blocks = append(tally.blocks, memberBlock{src: srcId, dst: dstId, engines: blockingEngines(result)})
		}
	}
	return tally
}

func blockingEngines(result ReachabilityResult) map[string]EngineVerdict {
	blockers := map[string]EngineVerdict{}
	for engine, engineVerdict := range result.Engines {
		if engineVerdict.Status == "deny" {
			blockers[engine] = engineVerdict
		}
	}
	return blockers
}
```

Categorize. Replace the current `verdict.Verdict != "deny"` guard + `Engines`
loop (`buildIssues.go:127-152`) with:

```go
if verdict.Verdict != "deny" {
	continue
}

srcIds, srcVerified := expandEndpoint(data, rule.SrcID, srcNode.Namespace)
dstIds, dstVerified := expandEndpoint(data, rule.DstID, dstNode.Namespace)

// Unverifiable: a ns endpoint we couldn't resolve to members. Don't guess —
// emit the coarse conflict flagged unverified rather than drop signal.
if !srcVerified || !dstVerified {
	issues = append(issues, coarseConflicts(verdict, &srcNode, &dstNode, false)...)
	continue
}

// Both endpoints resolved to concrete workloads — sweep the cross-product to
// tell a genuine block from a pure granularity artifact.
tally := sweepMembers(ctx, data, noMesh, srcIds, dstIds)
switch {
case tally.blocked == 0:
	// Artifact — the coarse deny was the ns-vs-pod mismatch. Drop, breadcrumb only.
	logger.Debug("policy issues: suppressed granularity artifact",
		slog.String("phase", "policy_issues"),
		slog.String("src", rule.SrcID),
		slog.String("dst", rule.DstID))
case tally.blocked == tally.total:
	issues = append(issues, fullConflicts(tally, &srcNode, &dstNode)...)
default:
	issues = append(issues, partialConflict(tally, &srcNode, &dstNode))
}
```

`coarseConflicts` / `fullConflicts` are today's `Engines`-loop issue builder
(one `PolicyConflicts` per blocking engine, culprits off the block engine).
`coarseConflicts` takes an extra `verified bool` it stamps on the issue.

## Categorization rules (do not collapse, do not silently drop)

| Sweep result | Meaning | Action |
|---|---|---|
| all pairs deny | genuine full block | `PolicyConflicts` (existing), highest severity |
| some deny / some allow | genuine partial block | **new** `PolicyConflictPartial` — carry blocked member ids + culprits |
| all pairs allow | pure granularity artifact | drop; `logger.Debug` breadcrumb only |
| unverifiable (empty ns / index miss) | couldn't check | emit coarse conflict, flagged unverified — never drop |

- **Partial is a distinct type, not a severity notch below full.** Different
  meaning: full = path dead; partial = path half-dead and the ns→ns edge is
  lying about it. Istio opens the ns, k8s admits pod-a, pod-b is dark, operator
  sees one green edge. Carry the blocked member ids + their culprit `PolicyRef`s
  so the UI renders "3 of 12 members blocked" and names them. Highest-value
  output here — the class the coarse detector cannot express.
- **Dropping the artifact is safe** — it's a property of our rendering (we drew a
  ns node), not the cluster. Silent to the user, not to the logs: debug-log the
  triggering pair so a "phantom" claim is confirmable.
- **Empty-member-list ≠ no conflict.** Unverifiable must emit, flagged. Treating
  it as "all reach" is the swallowed-signal failure.

## Backend touchpoints (user implements)
- `internal/models/issues.go` — new `PolicyConflictPartial` IssueType const;
  `Issue` fields for blocked members (ids + per-member culprits) on partial.
- `internal/store/buildIssues.go` — `expandEndpoint`, `sweepMembers`,
  `blockingEngines`, `coarseConflicts` / `fullConflicts` / `partialConflict`
  builders; rewire `PolicyIssues:127-152`.
- Frontend (`ui/`) — render `PolicyConflictPartial`: "N of M blocked" + member
  list, reuse `CulpritActions`.

## Files
- `internal/store/buildIssues.go:87-157`
- `internal/store/reachability.go:128-149`
- `internal/policy/istio/buildRules.go:307-322`
- `internal/policy/k8spolicy/buildRules.go:230-267`
- `internal/models/node.go:39-42`
