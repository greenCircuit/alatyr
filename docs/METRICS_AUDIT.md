# Metrics Branch Audit

Branch: `49-metrics-enpoint-and-grafana-dashboard` vs `main`.
Reviewer persona: senior SRE / Go operator engineer.
Target reality: shared cluster, tens of namespaces, hundreds-to-thousands of pods.

Findings ordered by severity (Critical → Low).

---

## Findings

### 1. Dual `/metrics` route registration — dead code that misleads auditors

- **Severity:** High
- **Location:** `internal/api/api.go:55-61` and `main.go:141`
- **Justification:** `RegisterRoutes` at `api.go:60` registers `/metrics` on the main Echo (port 8080), guarded by `s.metrics != nil`. `main.go:141` also registers `/metrics` on a separate `metricsEcho` (port 8085). Comment at `api.go:55-58` explicitly says "serving on the same Echo for v1; move to a separate listener when the ops story demands it." That move already happened in `main.go`, but the `api.go` registration was not removed. Result: `/metrics` is live on both port 8080 (behind Recover + logging middleware, on the same listener as the UI) and port 8085 (the intended dedicated listener). The port-8080 endpoint is undocumented, not the one the ServiceMonitor scrapes, and will double-count every scrape against the HTTP request histogram via the middleware it was explicitly designed to avoid. The comment in `main.go:129-132` explains exactly why this is wrong, then leaves the old one alive.
- **How it should be done instead:** Delete the `if s.metrics != nil { e.GET("/metrics", ...) }` block from `api.go:RegisterRoutes` entirely. Dedicated listener in `main.go` is correct; `api.go` block is leftover.
- **User impact:** Anyone who hits port 8080 `/metrics` (e.g., `kubectl port-forward` for a quick check) gets a valid but subtly different scrape — Recover middleware wraps it, HTTP-metrics middleware records `/metrics` as a high-frequency route, polluting `alatyr_http_requests_total` and `alatyr_http_request_duration_seconds` with self-referential noise. SREs reading the `path` breakdown see `/metrics` as busiest route.

---

### 2. `alatyr_mesh_namespaces_partially_enrolled` never resets — phantom value persists after mesh teardown

- **Severity:** High
- **Location:** `internal/metrics/snapshot.go:38-60` (`resetSnapshotVecs`), `internal/metrics/snapshot.go:383`
- **Justification:** Every other snapshot-derived gauge lives in a `GaugeVec` and gets wiped by `.Reset()` in `resetSnapshotVecs`. `meshNsPartial` is a plain `prometheus.Gauge` (not a vec, no labels), so cannot be `.Reset()`-ed — no label tuples to delete. Code calls `.Set(float64(cache.MeshMetrics.NsPartial))` on every cycle at line 383, overwriting on success. But `resetSnapshotVecs` does not include it, and the comment at `snapshot.go:36` lists explicit exclusions ("Counters + histograms + long-lived gauges (engine_enabled, build_info, informer_cache_synced, evaluation_timestamp)"). `meshNsPartial` isn't in that list either — just missing. If mesh is fully torn down mid-cycle (all namespaces deleted), gauges in `meshWorkloads` and `meshMtls` correctly vanish because their vecs are reset, but `meshNsPartial` stays at whatever its last value was until the next successful snapshot. Only scenario where this fires is a cluster-wide mesh removal, but that's exactly the scenario someone is on-call for.
- **How it should be done instead:** `.Set(0)` on `meshNsPartial` at the top of `resetSnapshotVecs`, or add it explicitly to the exclusion comment with rationale. One-liner: `r.meshNsPartial.Set(0)` in `resetSnapshotVecs` before `recordMesh` re-populates.
- **User impact:** `alatyr_mesh_namespaces_partially_enrolled` stays elevated after condition clears. SRE gets paged by the alert in the docs (`sum > 0`), investigates, sees no partial enrollments in the UI — concludes exporter lies. Trust erodes in exactly the metric this whole project exists to build.

---

### 3. Metrics listener has no `WriteTimeout` or `IdleTimeout` — scraper can hold connections open indefinitely

- **Severity:** High
- **Location:** `main.go:138-143`
- **Justification:** `metricsEcho.Server.ReadHeaderTimeout = 5 * time.Second` is set, but `WriteTimeout` and `IdleTimeout` are not. `net/http.Server` zero-value for these is "no timeout." A Prometheus scrape that stalls mid-response (dead scraper process, network partition leaving TCP session half-open) holds a goroutine open forever. On a busy cluster with 15s scrape interval and multi-second collection, stalled connections accumulate and eventually exhaust the goroutine pool. Not theoretical: `promhttp.HandlerFor` does a full collect before writing anything, so write-side stalls happen after entire metric set is computed but before flush. `go_goroutines` climbs silently.
- **How it should be done instead:** Add `WriteTimeout: 30 * time.Second` and `IdleTimeout: 90 * time.Second` alongside `ReadHeaderTimeout`. Echo exposes via `metricsEcho.Server.WriteTimeout` / `metricsEcho.Server.IdleTimeout`. 30s write timeout matches typical Prometheus scrape timeout defaults.
- **User impact:** Leaking goroutines until process OOMs or gets restarted. Single-replica deployment (standard for this tool) means restart drops the cache, causing one full evaluation cycle delay before graph is usable again.

---

### 4. `alatyr_informer_resync_total` documented but not implemented

- **Severity:** Medium
- **Location:** `docs/METRICS.md:102`
- **Justification:** Docs table in section 5 lists `alatyr_informer_resync_total` (counter, `resource` label, "Watch churn — useful when debugging why evaluations got slow"). Grep across entire `internal/` tree finds zero references: no field in `Recorder`, no registration in `registerMetrics`, no `RecordResync` method. Metric does not exist. Not in registry, so `/metrics` will never emit it. Docs say "watch churn" is "useful when debugging why evaluations got slow" — exact scenario where SRE turns to the dashboard and finds series absent.
- **How it should be done instead:** Either add the counter (`NewCounterVec("informer_resync_total", ..., "resource")` in `registerMetrics`, `RecordInformerResync(resource string)` method, call-sites on informer factory's resync events), or remove it from the docs. Docs-only gap is worse than not documenting it — creates false expectation.
- **User impact:** SRE adds `rate(alatyr_informer_resync_total[5m])` to their runbook for "evaluation getting slow" investigations. Never returns data. They assume cluster has no resync churn when it actually does.

---

### 5. `PrometheusRule` alerts documented but no chart template ships them

- **Severity:** Medium
- **Location:** `docs/METRICS.md:135-175`, `chart/templates/` (directory listing), `chart/values.yaml:63-64`
- **Justification:** `docs/METRICS.md` dedicates a "Starter alerts" section with concrete PromQL rules and explicitly says "Ship these as a `PrometheusRule` in the chart." `chart/values.yaml:63` has `alerts: enabled: true`. No `PrometheusRule` template exists under `chart/templates/`. The `alerts.enabled` flag is wired to nothing — not referenced by any template. Staleness alerts (`time() - alatyr_evaluation_timestamp_seconds > 900`) are the ones that make the rest of the dashboard trustworthy; without them, "zero internet-exposed workloads" and "the exporter stopped running" look identical.
- **How it should be done instead:** Add `chart/templates/prometheusrule.yaml` gated on `{{- if .Values.alerts.enabled }}`, containing the five staleness + findings rules from docs verbatim. PromQL is already written. Template copy, not new logic.
- **User impact:** Operators who install the chart expecting the promised alerts get nothing. Discover this the first time exporter silently stops evaluating — exact failure mode the alerts exist to catch.

---

### 6. `alatyr_workloads_internet_reachable` drops to zero silently — gauge not emitted vs emitting zero are indistinguishable from PromQL

- **Severity:** Medium
- **Location:** `internal/metrics/snapshot.go:114-123`
- **Justification:** Code skips `WithLabelValues(...).Set(0)` for directions with zero count — only emitting series when `internetIn > 0`, `internetOut > 0`, or `internetBoth > 0`. After `resetSnapshotVecs` clears the vec, a direction dropping from N to 0 has its series vanish entirely (tombstoned, not zeroed). Intentional for cardinality (don't emit series that aren't needed), but breaks the alert in `METRICS.md:163`: `increase(alatyr_workloads_internet_reachable{direction="both"}[1h]) > 0`. If series disappears, `increase()` returns no data, not 0 — alert never fires when situation resolves and re-appears within lookback window because no baseline to compute increase against. The `or vector(0)` pattern used in dashboard panels compensates in Grafana, but raw alert rules don't have that protection. Doc's own alert example omits `or vector(0)`.
- **How it should be done instead:** Either emit all three direction series unconditionally (including zeros) after reset, or add `or vector(0)` to the alert rule. Alert in docs at `METRICS.md:163` needs the fix regardless. Emitting zeros is lower-maintenance — makes series continuous, which is what `increase()` needs.
- **User impact:** Alert `increase(alatyr_workloads_internet_reachable{direction="both"}[1h]) > 0` silent when internet-facing workloads return after being removed. 24h-delta stat panel in dashboard (panel id 4, refId B) has `or vector(0)` guard so Grafana shows correctly, but PrometheusRule alert (once it exists) will be broken.

---

### 7. `METRICS_DETAIL` and `METRICS_ISSUE_POLICY_BREAKDOWN` not surfaced in the chart — operators can't configure them

- **Severity:** Medium
- **Location:** `chart/templates/deployment.yaml:34-38`, `chart/values.yaml`
- **Justification:** `main.go:77-79` reads `METRICS_DETAIL` and `METRICS_ISSUE_POLICY_BREAKDOWN` from env. `chart/templates/deployment.yaml` hard-codes `METRICS_PORT` in the env block but has no entries for the other two. No corresponding `values.yaml` key. Operators running on a large cluster who need to flip `METRICS_ISSUE_POLICY_BREAKDOWN=false` to avoid cardinality explosion have no Helm-native way — must reach for `env` overrides via a manual extraEnv pattern (chart doesn't even support that — no `extraEnv` in the template). `docs/METRICS.md:29` explicitly calls out `--metrics-detail=workload` as something operators choose; chart makes it unreachable.
- **How it should be done instead:** Add `metrics.detail: ""` and `metrics.issuePolicyBreakdown: ""` to `values.yaml`, then project them as env vars in the deployment template alongside `METRICS_PORT`. Empty string preserves existing defaults (nil pointer for breakdown, empty for detail).
- **User impact:** Operator on a 5,000-pod cluster reads docs, sees cardinality warning, wants to disable `issues_by_policy` — has to patch Deployment directly rather than use `helm upgrade --set`. Any subsequent `helm upgrade` silently reverts the env var.

---

### 8. ServiceMonitor scrapes dedicated port (8085) but `/metrics` on port 8080 is also live — scrape misconfiguration invisible

- **Severity:** Low
- **Location:** `chart/templates/serviceMonitor.yaml:20`, `internal/api/api.go:60`
- **Justification:** Consequence of dual-registration finding (#1). ServiceMonitor correctly points at `port: metrics` (8085). But until `api.go:60` is removed, any operator who points a scrape job at main service port 8080 path `/metrics` gets a valid but uncounted (by ServiceMonitor) scrape. Real risk is accidental double-scraping if someone naively adds a static scrape target for port 8080 while debugging.
- **How it should be done instead:** Fix root cause (remove `api.go:60`). No chart change needed once done.
- **User impact:** Low on its own, but compounds dual-registration finding — two valid endpoints with identical data make it unclear which is authoritative.

---

### 9. `evalTimestamp.Set` uses `time.Now()` inside `RecordSnapshot`, not actual evaluation completion time

- **Severity:** Low
- **Location:** `internal/metrics/snapshot.go:32`
- **Justification:** `RecordSnapshot` is called by `recordSnapshot` in `cache-refresh.go:83`, called after cache swap at `RefreshAll:39`. Actual evaluation completed before lock at `RefreshAll:35`. Gap between evaluation completion and `RecordSnapshot` call includes mutex lock/unlock plus `GetIssues` computation (`cache-refresh.go:82`). On a large cluster where `GetIssues` is expensive, this overstates freshness by however long issues take to compute. Alert `time() - alatyr_evaluation_timestamp_seconds > 900` will fire 900 seconds after last timestamp set — if issue computation takes 30s, effective stale window is `900 - 30 = 870s`, not 900. Minor, but measurable drift on clusters with thousands of issues.
- **How it should be done instead:** Capture `evaluatedAt := time.Now()` immediately after `populateErr` is nil in `RefreshAll`, pass into `recordSnapshot`, use it in `RecordSnapshot` instead of calling `time.Now()` again. Change lands in `cache-refresh.go` and `snapshot.go`'s `RecordSnapshot` signature.
- **User impact:** Staleness alert threshold effectively ~30s tighter than configured on large clusters. Not a pager event on its own, but worth knowing when tuning the 900s threshold.

---

## Conclusion

Branch ships useful observability but three High-severity gaps break its own promise of trust:

1. **Two live `/metrics` endpoints** (finding #1) — the exporter self-pollutes the HTTP histogram on port 8080, and there's now ambiguity about which port is authoritative. Pure cleanup, one delete.
2. **Silent goroutine leak on stuck scrapes** (finding #3) — missing `WriteTimeout` / `IdleTimeout` on the metrics listener eventually OOMs the single-replica pod. Two lines.
3. **`meshNsPartial` never resets** (finding #2) — the one metric guaranteed to lie in the exact scenario an operator would pull up the dashboard to investigate. One line.

The Medium findings cluster around a shared theme: **the docs describe a system more complete than the code ships.** `alatyr_informer_resync_total` doesn't exist (#4). The `PrometheusRule` chart template doesn't exist (#5). The `alerts.enabled` values key is wired to nothing (#5). Two cardinality-control env vars can't be set via Helm (#7). The staleness/increase alerts written in the docs will fire incorrectly against the metrics as emitted (#6). None of these are hard to fix — the docs already contain most of the source material — but shipping the branch as-is means the runbook in `docs/METRICS.md` is aspirational, not operational.

Fix order for minimum production risk:
1. Delete `api.go:60` (finding #1 + #8, one edit).
2. Add `.Set(0)` for `meshNsPartial` in `resetSnapshotVecs` (finding #2, one line).
3. Add `WriteTimeout` / `IdleTimeout` in `main.go:138-143` (finding #3, two lines).
4. Ship the `PrometheusRule` template + fix the `increase()` alerts with `or vector(0)` (findings #5 + #6).
5. Surface `METRICS_DETAIL` / `METRICS_ISSUE_POLICY_BREAKDOWN` in `values.yaml` + deployment template before anyone runs this against a >1000-pod cluster (finding #7).
6. Either implement `informer_resync_total` or delete it from the docs (finding #4).
7. Tighten `evalTimestamp` capture point (finding #9) if you care about accurate staleness at high issue counts.

Steps 1–3 are single-digit line changes and remove the three ways this branch can quietly page someone. Do those before merge.
