# Prometheus metrics

Metric names use the `alatyr_` prefix. **Metric names are permanent in practice**:
once someone writes an alert or a dashboard panel against one, renaming it breaks
everything downstream silently. The prefix is settled now, which is the right
order — `/metrics` should not ship before the name does.

Units follow Prometheus convention: `_seconds`, `_bytes`, `_total` for counters,
`_timestamp_seconds` for absolute times.

## Prerequisite

Findings metrics require a **full-cluster evaluation on a fixed interval**,
decoupled from HTTP requests. Prometheus scrapes carry no `?namespaces=` scope, so
the per-request evaluation model cannot serve them. The background snapshot loop
is a hard dependency, not an optimization — and once it exists the UI reads the
same snapshot and gets faster as a side effect.

## Cardinality budget

The failure mode to avoid: a monitoring tool that takes down the monitoring stack.

- **Default labels are `namespace` and one dimension key.** Never workload name,
  never policy name, never CIDR values, never port numbers.
- Aggregate series count is roughly `namespaces × distinct keys` — tens to
  hundreds of series on a normal cluster.
- Per-workload detail lives behind `--metrics-detail=workload`, **off by
  default**, documented as high-cardinality with an explicit warning in the
  chart's values file.
- On a 5,000-pod cluster, per-workload status gauges would be ~65,000 series from
  one exporter. That is the number to keep in mind when tempted to add a label.

---

## 1. Exposure and posture

The headline findings. These are what a dashboard is built around.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_workloads` | gauge | `namespace` | Denominator for every ratio below. Without it, percentages can't be computed in PromQL. |
| `alatyr_workloads_by_status` | gauge | `namespace`, `status` | One series per status key present. `status` is the slug (`internet-full`, `lan-egress`, `air-gapped`, …), never the glyph. |
| `alatyr_workloads_unpoliced` | gauge | `namespace` | Selected by zero engines — default-open. The single most alertable number the tool produces. |
| `alatyr_workloads_internet_reachable` | gauge | `namespace`, `direction` | `direction` = `ingress`/`egress`/`both`. Redundant with `_by_status` but worth having explicitly: it's the number people alert on, and a dedicated series survives status-key catalog changes. |

## 2. Coverage

Answers "is the policy layer actually doing anything here," which is different
from "is anything exposed."

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_workloads_covered` | gauge | `namespace`, `engine` | Workloads selected by at least one policy from this engine. **Not summable across engines** — a workload covered by two engines appears in both series, so `sum by (namespace)` overcounts. Read one engine at a time, or use `_unpoliced` / `_single_engine_coverage` for intersection views. |
| `alatyr_workloads_single_engine_coverage` | gauge | `namespace` | Covered by exactly one engine — a silent-failure risk, since the intersection collapses to that engine's opinion alone. |
| `alatyr_policies` | gauge | `namespace`, `engine`, `action` | Policy object counts. Cheap, and useful for spotting a GitOps sync that silently dropped a directory. Cluster-scoped policies (Calico GlobalNetworkPolicy, etc.) are stamped `namespace="_cluster"` so they don't render as a phantom empty namespace on the dashboard — the tool exists to surface cluster-scoped policy, so it must not disappear from its own exporter. |

## 3. Issues

One series per issue type per namespace. Maps directly onto the detectors already
in `buildIssues.go`.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_issues` | gauge | `namespace`, `type` | `type` uses the wire values: `no dns`, `policy conflict`, `partial access`, `cidr scope mismatch`, `mesh conflict`, `mesh transport blocked`, `mesh policy`, `failed to fetch`. |
| `alatyr_issues_by_engine` | gauge | `namespace`, `type`, `engine` | Only for issue types with engine attribution (policy conflict follows the *blocker*, not the permitter). Skip for issue types where engine is meaningless. |
| `alatyr_issues_by_policy` | gauge | `namespace`, `type`, `engine`, `policy_namespace`, `policy_name` | One series per culprit policy per issue type per namespace. Fans out each `Issue` across its ingress + egress + node culprit refs (deduped per issue). `engine` prefers the culprit's `Source` so cross-engine issues (policy conflict, cidr scope mismatch) stay attributed. Cluster-scoped culprits stamp `policy_namespace="_cluster"`. Registered by default. Disable via `METRICS_ISSUE_POLICY_BREAKDOWN=false` when the culprit fan-out on a cluster exceeds the scrape budget. |

Gauges, not counters — these are a current-state census, not an event stream. An
issue that persists across scrapes is the same issue.

## 4. Mesh

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_mesh_workloads` | gauge | `namespace`, `enrolled` | `enrolled` = `true`/`false`. Ambient membership. |
| `alatyr_mesh_namespaces_partially_enrolled` | gauge | — | Enrolled namespaces containing at least one opted-out workload. The rollout footgun, as a single number. |
| `alatyr_mesh_mtls_workloads` | gauge | `namespace`, `mode` | `mode` = `strict`/`permissive`/`disabled`/`unset`/`unknown`. Resolved verdict, not declared. |
| `alatyr_mesh_hbone_blocked_workloads` | gauge | `namespace` | Ambient workloads whose policy strips port 15008. Silent traffic loss; deserves its own series rather than living only under `alatyr_issues`. |

`unknown` in the mTLS breakdown means a PeerAuthentication fetch failed — it is a
health signal wearing a posture label. Alert on it.

---

## 5. Freshness and health

**Ship these in the same release as sections 1–4, not later.** Without them, "zero
internet-exposed workloads" and "the Calico engine stopped responding" render as
the same green panel. A findings dashboard with no staleness signal is actively
misleading, which is worse than no dashboard.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_evaluation_timestamp_seconds` | gauge | — | Unix time of the last completed full evaluation. Alert on age, not on the value. |
| `alatyr_engine_last_success_timestamp_seconds` | gauge | `engine` | **Per engine.** One engine failing while others succeed is the case the product is currently silent about. |
| `alatyr_engine_errors_total` | counter | `engine`, `reason` | Keep `reason` to a small closed set (`fetch`, `decode`, `unsupported`, `timeout`) — never raw error strings. |
| `alatyr_engine_enabled` | gauge | `engine` | 0/1. Distinguishes "Calico absent from this cluster" from "Calico broken." Without it every alert needs a hand-maintained engine list. |
| `alatyr_evaluation_duration_seconds` | histogram | — | Full-cluster evaluation. Drives the "is the interval sustainable" question. |
| `alatyr_engine_evaluation_duration_seconds` | histogram | `engine` | Which engine is the slow one. |
| `alatyr_evaluation_failures_total` | counter | — | Whole evaluations that produced no usable snapshot. |
| `alatyr_informer_cache_synced` | gauge | `resource` | 0/1 per GVK. Cache desync is invisible today and produces confidently wrong output. |
| `alatyr_informer_resync_total` | counter | `resource` | Watch churn — useful when debugging why evaluations got slow. |

## 6. Process and HTTP

Standard, and free.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `alatyr_build_info` | gauge | `version`, `commit`, `go_version` | Always 1. Standard pattern — lets a dashboard show which version produced the data. |
| `alatyr_http_requests_total` | counter | `path`, `code` | `path` = route pattern, never a raw URL with query params. |
| `alatyr_http_request_duration_seconds` | histogram | `path` | |
| Go runtime + process collectors | — | — | Free from the Prometheus client library. Include them. |

## 7. Opt-in detail (`--metrics-detail=workload`)

Off by default. Documented as high-cardinality.

| Metric | Type | Labels |
|---|---|---|
| `alatyr_workload_status` | gauge | `namespace`, `workload`, `status` |
| `alatyr_workload_issues` | gauge | `namespace`, `workload`, `type` |

Use `workload` from the owner-reference rollup (Deployment/StatefulSet/CronJob),
never a pod name. Pod names carry ReplicaSet hashes, so every rollout would churn
the entire series set and make the metrics useless for exactly the
before-and-after comparison someone would enable them for.

---

## Starter alerts

Ship these as a `PrometheusRule` in the chart. Most projects ship `/metrics` and
stop; the ones that get adopted ship the rules and the dashboard.

**Staleness first.** These gate the trustworthiness of everything else, so they
belong at the top of the rules file:

```
# Evaluation has stopped
time() - alatyr_evaluation_timestamp_seconds > 900

# One engine is failing while the others are fine
alatyr_engine_enabled == 1
  and time() - alatyr_engine_last_success_timestamp_seconds > 900

# Cluster reads are degraded
alatyr_informer_cache_synced == 0

# PeerAuthentication fetch is failing — mTLS posture is a guess
sum(alatyr_mesh_mtls_workloads{mode="unknown"}) > 0
```

**Findings.** Intentionally conservative — every one of these is a state that
should be zero in a healthy cluster, so they don't need thresholds tuned per site:

```
# A workload with no policy at all is default-open
alatyr_workloads_unpoliced > 0

# New internet exposure
increase(alatyr_workloads_internet_reachable{direction="both"}[1h]) > 0

# Engines disagree — one allows what another denies
alatyr_issues{type="policy conflict"} > 0

# Ambient traffic silently dropped
alatyr_mesh_hbone_blocked_workloads > 0

# Locked egress with no DNS
alatyr_issues{type="no dns"} > 0
```

Namespace allowlisting belongs in the alert's label matchers, not in the exporter.
Keep the exporter opinion-free.

## Shipping checklist

- `ServiceMonitor` in the chart, disabled by default, with a `values.yaml` toggle
- `PrometheusRule` with the alerts above, also toggleable
- Grafana dashboard JSON committed to the repo — the Cluster Status page is
  already the design; port the panel layout rather than inventing a new one
- `/metrics` documented as unauthenticated and cluster-revealing: exposure counts
  per namespace are reconnaissance data, so the same bind-to-localhost and
  network-policy guidance that applies to the UI applies here
