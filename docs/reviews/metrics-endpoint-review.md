# Prometheus `/metrics` endpoint — plan + backend-sre review

Source: backend-sre review of the proposed metrics endpoint design (2026-08-12).
Status: plan approved with three corrections. Not implemented.

---

## 1. Problem

There is no way to detect, without a human looking at the UI, that the graph has
gone wrong — an engine returning zero rules after a CRD change renders as "cluster
is secure", not as an error. A scrape endpoint fixes that, but only if the numbers
it exports are cluster-wide and honest.

Two things block a naive `/metrics` today:

- **Cache scope is request-shaped.** `store/buildStore.go:180` does
  `cache.EvaluationResults[source.Name()] = result` — wholesale replace per engine.
  A namespace-scoped `/api/graph` request narrows every engine's rule set while
  `cache.NsIndex` keeps the previously-fetched namespaces. Cache ends up internally
  inconsistent: nodes cluster-wide, rules from whatever the last browser session
  selected. `ClusterMetrics` (`api/metrics.go:19`) already serves "cluster totals"
  off that possibly-narrowed cache, and `/api/issues` has the same scope lie
  (`docs/issues/issues-feature-review.md:162`).
- **Nothing builds the cache without a browser.** Only `handleGraph`
  (`api/network-policy.go:26`) calls `PopulateCache`. A scrape after restart reads
  an empty cache and reports zeros, which alerts as "all policy disappeared".

Non-issue, checked: apiserver cost. Reads go through shared informers with resync
period 0 (`k8s/informerClient.go:74`), so rebuild frequency does not translate into
apiserver load. Per-cycle cost is in-process CPU and allocation only.

---

## 2. Approach — invert cache ownership

A background refresher owns the cache; HTTP handlers become read-only over an
atomically published snapshot.

This is not extra machinery. `graph.BuildGraph` (`graph/buildGraph.go:12`) is
already pure over the cache and already takes `namespaces` for per-request
narrowing, so the read path collapses to snapshot + build. It also makes the
wholesale replace at `buildStore.go:180` *correct*, because scope becomes
invariantly cluster-wide.

Explicitly rejected: a per-ns merge into a long-lived cache. `PolicyStatuses` and
`Nodes` are keyed by nodeID with no deletion signal, so merging leaks evicted
workloads forever.

Also rejected: rebuilding synchronously on each scrape. Even with in-memory
listers, a scrape rebuild re-runs every engine, mesh membership, and the
per-enrolled-node `ValidateExternalRules` loop (`buildStore.go:210-221`) inside the
scrape goroutine. Scrape cadence is not guaranteed — federation, a debugging `curl`
loop, or a second scrape config would stack full rebuilds with no coalescing.

### Sketch

```go
// internal/store/refresher.go
type Snapshot struct {
    Cache       *models.Cache
    EvaluatedAt time.Time
}

type Refresher struct {
    builder  *Builder
    client   k8s.KubernetesClient
    log      *slog.Logger
    interval time.Duration
    trigger  chan struct{}              // buffered cap 1, coalescing manual refresh
    current  atomic.Pointer[Snapshot]
}
```

`Run(ctx)`: tick → `GetNsNames()` → **fresh** `&models.Cache{}` → `PopulateCache` →
on success `current.Store(...)`. Build runs off any lock; only the pointer store is
shared. A failed cycle leaves the last good snapshot in place.

### Handler changes

- `api.Server`: drop `cache *models.Cache` and `mu sync.RWMutex`, add
  `refresher *store.Refresher`. The mutex disappears — `atomic.Pointer` replaces it.
- `handleGraph` (`api/network-policy.go:24-29`): delete the lock and the
  `PopulateCache` call; snapshot, nil-check, `graph.BuildGraph(snap.Cache, namespaces)`.
- Six other cache readers switch to the snapshot: `node-info`, `reachable`,
  `manifest`, `issues`, `meshStatus`, `ClusterMetrics`. Mechanical, but each needs
  the nil check.
- Cold start returns 503 `{"status":"building"}`. Never an empty graph — an empty
  graph renders as a cluster with no workloads and no policy, indistinguishable
  from a broken cluster.
- `main.go:64`: construct refresher, `go refresher.Run(ctx)`, pass into `api.New`.
  Interval from env, default 30s.

`DemoClient` needs no special case — same interface, loop re-reads fixtures.

**API contract:** `evaluatedAt` ships in the graph response in the same change.
Once builds go periodic, an unlabeled graph is of unknown age, and that is a trust
failure.

---

## 3. Corrections from review

### 3.1 In-place mutation vs. pointer swap — panic risk

`PopulateCache` mutates its argument in place: `cache.NsIndex[ns]`
(`buildStore.go:128`), `cache.EvaluationResults[...]` (`:180`),
`RebuildWorkloadIndex()` rewriting `WorkloadByID` (`:184`).

If the refresher reuses one `models.Cache` across ticks and just re-publishes the
pointer, the snapshot a reader holds and the one being built alias the same Go
maps → concurrent map read/write → **panic**, not stale data.

The refresher must allocate a new `&models.Cache{}` every tick and never touch a
published one. Nil-map init at `buildStore.go:91-96` already tolerates a fresh
empty struct, so this is refresher discipline, not a `PopulateCache` change. Put it
in the doc comment — this is the failure that looks fine in a demo and panics under
load later.

### 3.2 Overlapping ticks

Cluster-wide builds may exceed the interval. Fresh objects make that *safe* but not
*free* — N full engine evaluations in flight. Single-flight the loop (build-in-progress
guard), don't let ticks stack.

### 3.3 Partial-success is a control-flow change, not a signature change

Proposed:

```go
type PopulateResult struct {
    NsErrors     map[string]error
    EngineErrors map[string]error
    NsOK         int
}
func (b *Builder) PopulateCache(cache *models.Cache, namespaces []string) (PopulateResult, error)
```

But today a single engine failure hard-aborts everything after it —
`buildStore.go:158` skips remaining engines *and* the whole mesh block
(`:186-231`); `:200` does the same for mesh. Producing a real partial result means
collecting errors and continuing, not just widening the return type. Track it as
its own chunk of work.

Semantics: fatal → refresher skips the swap, keeps the last good snapshot. Partial
→ swap, record errors as metrics. Swap only when `NsOK > 0` and at least one engine
succeeded — a snapshot where every engine failed renders as "cluster has no policy",
the exact lie being designed against. `handleGraph` keeps today's fail-fast behavior.

### 3.4 Cold-start readiness — decide explicitly

`main.go:82` blocks in `e.Start()`. Either block on the first successful populate
before opening the port (safer for external health checks, delays readiness by one
full cluster-wide build) or open immediately and 503. Pick one deliberately rather
than inheriting whichever falls out of goroutine ordering.

---

## 4. The metric that was missing — informer health

**This is the one that would actually page someone, and the original plan did not
have it.**

`PopulateCache` never contacts the apiserver. It reads listers backed by
long-running watches (`informerClient.go:36-49`); `WaitForCacheSync` runs once at
startup (`:120-138`). If a watch dies afterwards — RBAC revoked mid-run, relist
failure, compacted resourceVersion — the lister serves its last-known state
forever, and every subsequent `PopulateCache` returns `nil` error with full `NsOK`.

Under the plan as written, `builds_total{result="success"}` stays green and
`last_success_timestamp_seconds` keeps advancing every 30s while the graph is
frozen. That is worse than a build failure, because nothing pages.

Fix, cheapest item in the whole plan:

- register client-go's reflector/rest-client collectors
  (`k8s.io/client-go/tools/metrics`) — list/watch error counters
- and/or export `informer.HasSynced()` per lister as
  `policyviz_informer_healthy{resource}`

---

## 5. Metric set

New package `internal/metrics/metrics.go`. Custom registry, not the default one —
explicit `collectors.NewGoCollector()` + `NewProcessCollector()`, avoids
panic-on-duplicate-registration when a library registers too.

### Refresher-owned (counters/histograms, incremented at build time)

```
policyviz_build_duration_seconds                histogram
policyviz_builds_total{result}                  counter   # success|partial|failed
policyviz_populate_errors_total{phase,engine}   counter   # phase=ns_index|engine_evaluate|mesh
policyviz_last_success_timestamp_seconds        gauge
```

### Scrape-time custom `prometheus.Collector` reading the snapshot

```
policyviz_namespaces_total
policyviz_workloads_total
policyviz_policies_total{source}
policyviz_rules_total{source,action}
policyviz_status_keys_total{key}
```

Two reasons over bookkeeping gauges: no drift between cache and metric, and — when
the snapshot is nil — **emit no samples at all**. Absent series alert as an outage
via `absent()`; a zero gauge alerts as a security regression that never happened.

Derive `status_keys_total` from the effective `Statuses`. If per-engine breakdown is
also wanted, use a separate metric name — two different numbers under one name is a
3am trap.

### Free win — mesh posture

`cache.MeshMetrics` (`models/mesh.go:85-94`) is already fully computed post-populate:
`NsEnrolled`, `WorkloadsEnrolled`, `MtlsStrict/Permissive/Disabled/Unset/Unknown`.
Exporting these as plain gauges costs no new computation and, on an ambient-mode
cluster mid-migration, has higher operational value than `status_keys_total` —
mTLS posture is exactly the blind spot L3/L4 tooling misses
(`docs/issues/issues-feature-review.md:54-66`).

### Cardinality — tiered, not blanket-banned

Three tiers, and the distinction is what makes Grafana drill-down (section 7)
possible without blowing up someone else's Prometheus:

- **Always safe** — `source` (3), `action` (2), `key` (fixed `policy.AllStatusKeys()`),
  `type`/`severity` (~9 issue types), `phase`, `result`. Bounded by code, not by
  cluster size.
- **Safe at target scale, with a cap** — `namespace`. Tens of namespaces is the
  stated target; ~13 status keys × tens of namespaces is a few hundred series,
  which is nothing, and it's the dimension audit actually needs. Guard it with a
  documented series cap (section 7) so a 500-namespace cluster degrades to the
  aggregate instead of melting.
- **Only where the data is sparse** — `workload`. Legal on issue detail series
  (one per finding, and findings are tens-to-hundreds) — never on status keys or
  rules, where it multiplies by every workload in the cluster.

**Never**: pod name (churns on every restart — worst kind of cardinality, high
*and* high-turnover), policy name, CIDR, port. Note that `WorkloadNode.ID`
(`models/node.go`) is owner-derived, not pod-derived, so workload labels are stable
across rollouts — that's what makes the sparse tier viable at all.

Put this tiering as a comment in `internal/metrics/metrics.go`; the next person
adding a dashboard will otherwise reach for pod name.

### Route

```go
e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
```

Register before `RegisterUI` so the SPA HTML5 fallback doesn't swallow it.

---

## 6. Issues by type and severity — in scope

This is the alerting payload. Everything in section 5 tells you the *tool* is
healthy; `issues_total` is the only metric that tells you the *cluster* isn't.
It ships with the rest, not after.

```
policyviz_issues_total{type,severity}
```

### Severity moves into Go — small, because it's a lookup

Issue severity is not derived in the UI. `ui/src/components/FilterPanel/parts/constants.ts:10`
is a static `Record<IssueType, Severity>` — a constant table keyed by issue type,
nothing per-instance. (Distinct from `STATUS_CFG` at `ui/src/store/clusterStats.ts:118`,
which maps *status keys*, not issue types — that one stays put.)

Port shape in `internal/models/issues.go`, next to the existing `IssueType` consts:

```go
type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityWarning  Severity = "warning"
    SeverityCaution  Severity = "caution"
    SeverityInfo     Severity = "info"
)

var issueSeverity = map[IssueType]Severity{ /* mirror of TYPE_SEVERITY */ }
```

Add `Severity Severity \`json:"severity"\`` to `Issue` and stamp it where issues are
assembled (`store/buildIssues.go:18` `GetIssues`, plus the mesh issue path that
appends into `cache.MeshIssues`) so it lands in both the API response and the metric
from one place. Then delete `TYPE_SEVERITY` and have the UI read `issue.severity`.

One dead entry to skip while porting: `NodeLockOut` (`models/issues.go:11`) is
declared but never constructed anywhere in the codebase. Don't carry a phantom into
the Go severity map — either delete the const or leave it out of the table.

Keeping the frontend table *and* adding a Go one is the thing to avoid — the alert
firing "critical" while the UI badge shows "warning" is exactly the drift
`docs/issues/issues-feature-review.md:154` warns about. Delete the TS map in the
same change.

### Cardinality

Severity is functionally dependent on type, so the label pair yields at most one
series per issue type — ~9 today (`models/issues.go:5-18`). Carrying `severity`
anyway is worth it: it lets an alert say `severity="critical"` without enumerating
issue types, so a newly added type is covered on day one instead of silently
missing from the rule.

### Alerting caveats — real, and they belong in the rule, not the metric

- **No stable issue identity** (`docs/issues/issues-feature-review.md:112-119`).
  Issues are recomputed from scratch each cycle, so a count is a level, not an
  event stream. Pod churn and rollouts make counts flap. Alert with a `for:`
  duration (several cycles) rather than on any nonzero reading, and alert on
  *increase* against a baseline for the noisy types rather than absolute count.
- **`IssuesFailedToFetch`** (`models/issues.go:17`) is not a cluster finding — it
  means the tool couldn't read something. Route it to the same alert family as
  `populate_errors_total` / informer health, not to the security alerts. Otherwise
  a broken watch reads as "cluster got worse".
- **Zero vs. absent applies here too.** `issues_total` must be emitted by the same
  snapshot-reading collector as section 5, so a nil snapshot produces no series.
  A hardcoded zero on an unbuilt cache reads as "all issues resolved".
- **Scope is already fixed** by section 2 — a cluster-wide snapshot removes the
  `/api/issues` scope lie (`docs/issues/issues-feature-review.md:162`), which is
  what made per-severity counts untrustworthy in the first place. Section 3.5 of
  that doc gates notifications on exactly this refresh loop existing.

### Suggested rules

```
policyviz_issues_total{severity="critical"} > 0            for: 5m
increase(policyviz_issues_total{severity="high"}[1h]) > 0  # new exposure appeared
policyviz_issues_total{type="failed to fetch"} > 0         # tool problem, not cluster
```

---

## 7. Grafana as a first-class consumer

Requirement: an operator paged at 3am must get from an alert to *which namespace,
which workload, which engine* without opening the graph UI. Grafana is an
alternative front end, not just a chart of section 5's aggregates.

That needs two things aggregates can't give: identity on the detail series, and a
way back into the UI when the operator does want the topology.

### 7.1 Two-tier issue metrics

Alerting and drill-down have opposite cardinality needs, so don't compromise —
emit both.

```
# tier 1 — alerting. always emitted, tiny.
policyviz_issues_total{type,severity}                        gauge

# tier 2 — drill-down. one series per finding, value always 1.
policyviz_issue_info{type,severity,namespace,workload,engine} gauge
```

`issue_info` is the standard info-metric pattern: value is meaningless, the labels
are the payload. A Grafana table panel over
`policyviz_issue_info{severity="critical"}` is the triage list — namespace,
workload, engine, issue type, no UI needed. Alerts stay on tier 1 so rule
evaluation never touches the wide series.

`Issue.Engine` (`models/issues.go:29`) and the `Node`/`Src`/`Dst` `*WorkloadNode`
pointers (`:34-36`) already carry everything the labels need — no new computation,
just projection at scrape time.

**Issue volume is workload-scaled, not sparse.** Do not size this on an assumption
of "tens of findings":

- `MissingDns` (`store/buildIssues.go:57-116`) iterates every cached workload as
  `src` and appends one `Issue` per (workload × engine) when DNS egress is blocked
  (`:93-101`, `:104-112`). One bad DNS-egress policy — the single most common real
  incident this endpoint exists to catch — produces **one issue per pod per engine**.
  At hundreds-to-thousands of workloads and three engines, that's thousands of
  `issue_info` series from one rollout.
- `MeshTransportBlocked` has the same shape: `ValidateExternalRules` runs per
  enrolled workload (`store/buildStore.go:210-221`).
- `PolicyConflicts` / `MeshConflicts` (`store/buildIssues.go:159-321`) are keyed by
  `srcID + "-" + dstID` (`:170`), so they're bounded by distinct allow-rule pairs —
  O(N²) worst case in a dense mesh, not O(1).

So the cap is not a backstop for a rare pathological cluster; it is load-bearing on
the common path. Emit at most N detail series (start at 5000, env tunable), and when
it truncates, say so:

```
policyviz_issue_info_truncated  gauge   # 0 normally, else count of dropped findings
```

Silent truncation would make Grafana show a triage list that is confidently
incomplete — the same lying-graph failure mode this whole document is about. Tier 1
stays exact regardless, so the alert still fires with the true count even when the
detail list is clipped.

Truncate deterministically — sort by severity then type before clipping, so the
surviving series are the ones worth triaging and the list doesn't reshuffle between
scrapes. Random map-order truncation gives Grafana a table that changes rows every
15s.

### 7.2 Link back to the UI

Export a build-info metric carrying the UI base URL, so a Grafana panel can
template a deep link per row instead of hardcoding the host in every dashboard:

```
policyviz_build_info{version,ui_base_url}  gauge   # value 1
```

Combined with 7.1's labels, a Grafana data link becomes
`{{ui_base_url}}/?namespace={{namespace}}&node={{workload}}` — drill-down when the
operator wants the graph, self-sufficient triage when they don't.

### 7.3 Status-key rollups — the audit surface

`status_keys_total{key}` in section 5 answers "did an engine go blind". Audit asks
a different question: *where* is the exposure. Same collector, one more dimension:

```
policyviz_status_key_workloads{key,namespace}       gauge   # workloads in ns carrying key
policyviz_status_key_workloads_total{key}           gauge   # cluster rollup (section 5)
policyviz_workloads_by_namespace{namespace}         gauge   # denominator
```

The denominator is what makes it an audit rather than a number. `internet-egress`
count of 12 means nothing; 12 of 14 workloads in `payments` means something. Ratio
in Grafana, not in Go — don't precompute a rate that hides its own numerator.

Two more that come free from the same snapshot and answer the questions an auditor
actually asks first:

```
policyviz_workloads_unpoliced{namespace}   # workloads with empty Statuses — no engine
                                           # asserted anything about them
policyviz_workloads_by_engine{source,namespace}  # workloads with a non-empty
                                                 # StatusesBySource[source] entry
```

`unpoliced` is the coverage gap — the workloads nothing evaluated, which is the
blind spot a policy audit exists to find, and it's derivable directly from
`WorkloadNode.Statuses` being empty (`models/node.go`). `by_engine` off
`StatusesBySource` shows engine reach per namespace — "Calico covers 40 of 60
workloads in `prod`" is an adoption-gap answer no aggregate gives you.

**Filter by `node.Type` first — without it these metrics invert the truth.** Not an
edge case; it fires on every cluster, every scrape.

`cache.WorkloadByID` and `Graph.Nodes` are not "workloads". `NSIndex.Workloads`
includes the synthetic namespace node appended by `buildNsIndex`
(`models/node.go:44-49`), and `buildGraph.go:45-53` folds in engine-synthesized CIDR
peer nodes. Both pollute the rollups in opposite directions:

- **Namespace nodes never read as unpoliced, and always read as exposed.**
  `k8spolicy/buildStatusKeys.go:20-22` runs over every entry of
  `nsIndex.Workloads` — ns node included — and sets a `PolicyStatus` even when no
  policy matched (zero value). `DeriveStatusKeys` on a zero-value status computes
  `internetEgress := !EgressLocked || ...` → `true`, likewise ingress
  (`policy/statusKeys.go:18-26`), so it emits `internet-full`. `UpdateStatusKeys`
  (`graph/renderEngine.go:263-283`) doesn't special-case `NodeTypeNamespace`. Net
  effect: every namespace without a catch-all NetworkPolicy shows up as
  internet-exposed and as engine-covered, from an engine that evaluated nothing.
- **CIDR nodes always read as unpoliced.** They exist only in `result.Nodes`
  (`k8spolicy/evaluate.go:95`), never in `nsIndex.Workloads`, so no engine writes a
  `PolicyStatuses` entry; `UpdateStatusKeys` hits the empty-`bySource` `continue`
  (`renderEngine.go:267-269`) and `Statuses` stays nil. A naive collector counts
  every CIDR peer as a coverage gap, and buckets it under an empty-string namespace
  label.

The collector must restrict to real workload types —
`NodeTypeService`, `NodeTypeDeployment`, `NodeTypeHeadless`, `NodeTypeCronJob`
(`models/node.go:18-24`) — and treat `NodeTypeNamespace`, `NodeTypeCIDR`,
`NodeTypeExternal` as structurally excluded, not as "happens to be empty". Same
filter applies to the `workloads_by_namespace` denominator, or the ratio is wrong on
both sides.

This also means `status_key_workloads{key}` and the section 5
`status_keys_total{key}` must use the same filter, or the two disagree.

Cap these the same way as 7.1. At 13 keys (`policy/catalog.go`) × tens of namespaces
the math is fine, but say it explicitly rather than leaving the guard implied — the
namespace dimension is the one someone will later copy onto a per-workload metric.

Note these are *intent*, not effective behavior — same caveat the badges carry: a
per-engine `PolicyStatus` only flips dimensions the policy explicitly references.
Document that in the metric HELP text, because a Grafana panel has no tooltip to
explain it and "unpoliced" will otherwise be read as "unreachable".

### 7.4 HELP text is the documentation

Nobody reading a Grafana panel will open this file. The `HELP` string is the only
docs an operator gets, and `/metrics` is self-describing by design — use it. Every
metric gets a full sentence: what it counts, what scope, and the caveat if it has
one. "Workloads with no status key from any engine — nothing evaluated them; not
the same as unreachable" beats `# HELP policyviz_workloads_unpoliced unpoliced
workloads`.

---

## 8. Order of work

1. Refresher + snapshot inversion, cold-start 503, `evaluatedAt` in the graph
   response. Fresh cache per tick (3.1) is non-negotiable.
2. Informer health metrics (section 4) — smallest change, biggest alerting value.
3. `Severity` into `internal/models/issues.go` + stamped on `Issue` (section 6).
4. `internal/metrics` with the refresher counters + snapshot collector + mesh gauges
   + `issues_total{type,severity}`. HELP text written properly (7.4), not stubbed.
5. Drill-down tier: `issue_info` + truncation guard, `build_info` with UI URL (7.1, 7.2).
6. Status-key rollups with namespace dimension + `workloads_unpoliced` /
   `workloads_by_engine` denominators (7.3).
7. Partial-failure control flow (3.3) and `populate_errors_total`.

Frontend follow-up (separate change, unrestricted): handle 503 as "building, retry",
render `evaluatedAt` as an age, manual refresh hitting the trigger channel, and
delete `TYPE_SEVERITY` in favor of `issue.severity` from the API. Without the first
three, step 1 ships a UI that silently shows old data; without the last, the alert
and the badge can disagree.

Tests worth adding: refresher keeps the previous snapshot when a cycle fails; graph
handler 503s on a nil snapshot. Both are the failure modes that otherwise surface as
a confidently wrong graph.
