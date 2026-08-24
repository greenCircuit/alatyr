// Package metrics owns the Prometheus registry, all alatyr_* metric
// definitions, and the /metrics HTTP handler. Nothing else in the repo
// imports client_golang — engine, store, and api call Recorder methods
// and stay free of the dep.
//
// Ownership rules:
//   - Metric names + label sets are permanent once shipped. Renames are
//     silent breakage for every dashboard and alert downstream.
//   - Gauge vecs get .Reset() before repopulation each snapshot cycle.
//     Otherwise deleted namespaces leave phantom series forever.
//   - Never add workload/pod/policy-name as a default label. Per-workload
//     detail lives behind Config.Detail = DetailWorkload.
package metrics

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Recorder holds every metric vec + the underlying registry. Constructed once
// at process start; passed to every subsystem that needs to write a metric.
// Safe for concurrent use — all client_golang types are.
type Recorder struct {
	cfg      Config
	logger   *slog.Logger
	registry *prometheus.Registry

	// Section 1 — exposure and posture
	workloads                    *prometheus.GaugeVec
	workloadsByStatus            *prometheus.GaugeVec
	workloadsUnpoliced           *prometheus.GaugeVec
	workloadsUnpolicedExclGlobal *prometheus.GaugeVec
	workloadsInternetReach       *prometheus.GaugeVec

	// Section 2 — coverage
	workloadsCovered           *prometheus.GaugeVec
	workloadsSingleEngineCover *prometheus.GaugeVec
	policies                   *prometheus.GaugeVec

	// Section 3 — issues
	issues                     *prometheus.GaugeVec
	issuesByEngine             *prometheus.GaugeVec
	issuesByPolicy             *prometheus.GaugeVec
	workloadsWithIssues        *prometheus.GaugeVec
	workloadsWithIssuesByType  *prometheus.GaugeVec

	// Section 4 — mesh
	meshWorkloads          *prometheus.GaugeVec
	meshNsPartial          prometheus.Gauge
	meshMtls               *prometheus.GaugeVec
	meshHboneBlocked       *prometheus.GaugeVec

	// Section 5 — freshness and health
	evalTimestamp          prometheus.Gauge
	engineLastSuccess      *prometheus.GaugeVec
	engineErrors           *prometheus.CounterVec
	engineEnabled          *prometheus.GaugeVec
	evalDuration           prometheus.Histogram
	engineEvalDuration     *prometheus.HistogramVec
	evalFailures           prometheus.Counter
	informerCacheSynced    *prometheus.GaugeVec

	// Section 6 — process + HTTP
	buildInfo         *prometheus.GaugeVec
	httpRequests      *prometheus.CounterVec
	httpRequestDur    *prometheus.HistogramVec

	// Section 7 — opt-in per-workload detail. Nil unless Detail == DetailWorkload.
	workloadStatus *prometheus.GaugeVec
	workloadIssues *prometheus.GaugeVec

	// Section 8 — rules.
	// ruleEdges: raw post-expansion count. One podSelector:{} manifest fans
	// into src×dst rules, so this counts graph edges, not policy objects.
	// Sum-safe across any label subset.
	// policyCoverage: deduped by (engine, ns, name, action, coverage,
	// direction). One count per policy per bucket. sum() is NOT a valid total.
	ruleEdges      *prometheus.GaugeVec
	policyCoverage *prometheus.GaugeVec
}

// New constructs a Recorder with all metric vecs registered on a fresh
// registry (not the default one — keeps unrelated global state out of
// /metrics). Panics on registration collision, which only happens if the
// same Recorder is built twice against a shared registry — a programmer
// error, not a runtime condition worth handling.
func New(cfg Config, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	registry := prometheus.NewRegistry()
	recorder := &Recorder{
		cfg:      cfg,
		logger:   logger,
		registry: registry,
	}
	recorder.registerCollectors()
	recorder.registerMetrics()
	return recorder
}

// Handler returns the http.Handler serving /metrics for this recorder's
// registry. Wire it onto a mux or Echo route in main.
func (r *Recorder) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		Registry:          r.registry,
		EnableOpenMetrics: true,
	})
}

// Registry exposes the underlying registry for callers that need to register
// additional collectors (rare — kept out of the constructor to avoid a
// growing zoo of options).
func (r *Recorder) Registry() *prometheus.Registry { return r.registry }

func (r *Recorder) registerCollectors() {
	// Go runtime + process metrics ship with the canonical unprefixed names
	// (go_*, process_*) on purpose — every stock Grafana/Prometheus dashboard
	// keys on them literally. Prefixing with "alatyr_" would break those
	// dashboards for a purely cosmetic gain. Only alatyr-owned metrics carry
	// the namespace.
	r.registry.MustRegister(collectors.NewGoCollector())
	r.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

func (r *Recorder) registerMetrics() {
	factory := promauto{registry: r.registry}

	// Section 1
	r.workloads = factory.NewGaugeVec("workloads",
		"Workloads observed in the last snapshot. Denominator for every ratio.",
		"namespace")
	r.workloadsByStatus = factory.NewGaugeVec("workloads_by_status",
		"Workloads carrying a given effective status key. status is the slug (internet-full, lan-egress, ...).",
		"namespace", "status")
	r.workloadsUnpoliced = factory.NewGaugeVec("workloads_unpoliced",
		"Workloads selected by zero policy engines — default-open. Pinned at 0 on any cluster with a cluster-wide catch-all (Calico GlobalNetworkPolicy): use workloads_unpoliced_excluding_global there.",
		"namespace")
	r.workloadsUnpolicedExclGlobal = factory.NewGaugeVec("workloads_unpoliced_excluding_global",
		"Workloads with no NAMESPACE-LOCAL policy selecting them. Cluster-scoped manifests (Calico GlobalNetworkPolicy) don't count as coverage — one global catch-all otherwise marks every pod covered and hides workloads with no policy of their own. Always >= workloads_unpoliced.",
		"namespace")
	r.workloadsInternetReach = factory.NewGaugeVec("workloads_internet_reachable",
		"Workloads reachable to or from the internet, broken down by direction (ingress|egress|both).",
		"namespace", "direction")

	// Section 2
	r.workloadsCovered = factory.NewGaugeVec("workloads_covered",
		"Workloads selected by at least one policy from this engine.",
		"namespace", "engine")
	r.workloadsSingleEngineCover = factory.NewGaugeVec("workloads_single_engine_coverage",
		"Workloads covered by exactly one engine — silent-failure risk on intersection.",
		"namespace")
	r.policies = factory.NewGaugeVec("policies",
		"Distinct policy manifests per engine per namespace. Deduped by (engine, ns, name). Action is a rule-level attribute — Calico/Kyverno mix allow+deny in one manifest, so keying by action would double-count. Action, direction, coverage live on alatyr_rule_edges.",
		"namespace", "engine")

	// Section 3
	r.issues = factory.NewGaugeVec("issues",
		"Current issues per type per namespace. Gauge, not counter — a persistent issue is the same issue.",
		"namespace", "type")
	r.issuesByEngine = factory.NewGaugeVec("issues_by_engine",
		"Issues attributed to a specific engine. Not every issue type carries engine attribution.",
		"namespace", "type", "engine")
	r.workloadsWithIssues = factory.NewGaugeVec("workloads_with_issues",
		"Distinct workloads with at least one finding, per namespace. Blast-radius denominator counterpart to alatyr_issues (which counts findings and double-counts workloads hit by multiple issues). Ratio over alatyr_workloads = % of namespace affected.",
		"namespace")
	r.workloadsWithIssuesByType = factory.NewGaugeVec("workloads_with_issues_by_type",
		"Distinct workloads with at least one finding of the given type, per namespace. Sum across types is NOT a workload count — a workload hit by two types counts once per type.",
		"namespace", "type")

	// Section 4
	r.meshWorkloads = factory.NewGaugeVec("mesh_workloads",
		"Workloads by mesh enrollment (enrolled=true|false).",
		"namespace", "enrolled")
	r.meshNsPartial = factory.NewGauge("mesh_namespaces_partially_enrolled",
		"Enrolled namespaces containing at least one opted-out workload. The rollout footgun as a single number.")
	r.meshMtls = factory.NewGaugeVec("mesh_mtls_workloads",
		"Mesh workloads by resolved mTLS verdict (strict|permissive|disabled|unset|unknown). unknown means a PA fetch failed — alertable.",
		"namespace", "mode")
	r.meshHboneBlocked = factory.NewGaugeVec("mesh_hbone_blocked_workloads",
		"Ambient workloads whose policy strips port 15008 (HBONE). Silent traffic loss.",
		"namespace")

	// Section 5
	r.evalTimestamp = factory.NewGauge("evaluation_timestamp_seconds",
		"Unix time of the last completed full evaluation. Alert on age of this value.")
	r.engineLastSuccess = factory.NewGaugeVec("engine_last_success_timestamp_seconds",
		"Unix time of the last successful evaluation per engine. One engine failing while others succeed is the case the product is otherwise silent about.",
		"engine")
	r.engineErrors = factory.NewCounterVec("engine_errors_total",
		"Engine evaluation errors, bucketed by reason. reason is a small closed set — never a raw error string.",
		"engine", "reason")
	r.engineEnabled = factory.NewGaugeVec("engine_enabled",
		"1 when the engine is enabled in this build/cluster, 0 otherwise. Distinguishes absent from broken.",
		"engine")
	r.evalDuration = factory.NewHistogram("evaluation_duration_seconds",
		"Full-cluster evaluation duration.",
		prometheus.DefBuckets)
	r.engineEvalDuration = factory.NewHistogramVec("engine_evaluation_duration_seconds",
		"Per-engine evaluation duration — pinpoints which engine dominates cycle time.",
		prometheus.DefBuckets,
		"engine")
	r.evalFailures = factory.NewCounter("evaluation_failures_total",
		"Whole evaluations that produced no usable snapshot.")
	r.informerCacheSynced = factory.NewGaugeVec("informer_cache_synced",
		"1 when the shared informer for a GVK has synced, 0 otherwise. Cache desync produces confidently wrong output.",
		"resource")

	// Section 6
	r.buildInfo = factory.NewGaugeVec("build_info",
		"Always 1. Labels carry version, commit, and go_version so dashboards can show which build produced the data.",
		"version", "commit", "go_version")
	r.httpRequests = factory.NewCounterVec("http_requests_total",
		"HTTP requests served. path is the route pattern, never a raw URL with query params.",
		"path", "code")
	r.httpRequestDur = factory.NewHistogramVec("http_request_duration_seconds",
		"HTTP request duration.",
		prometheus.DefBuckets,
		"path")

	// Section 7 — opt-in high-cardinality. Register only when enabled so
	// scraping doesn't even list the names when off.
	if r.cfg.Detail == DetailWorkload {
		r.workloadStatus = factory.NewGaugeVec("workload_status",
			"Per-workload effective status. High cardinality — off by default.",
			"namespace", "workload", "status")
		r.workloadIssues = factory.NewGaugeVec("workload_issues",
			"Per-workload issues. High cardinality — off by default.",
			"namespace", "workload", "type")
	}

	// Culprit-policy issue breakdown. On by default because the fan-out is
	// bounded by the number of policies actually cited as culprits (not the
	// full policy inventory). Disable via Config.IssuePolicyBreakdown when
	// running against clusters with pathological culprit counts.
	if r.cfg.IssuePolicyBreakdownEnabled() {
		r.issuesByPolicy = factory.NewGaugeVec("issues_by_policy",
			"Issue counts attributed to a specific culprit policy. Enabled by default; disable via METRICS_ISSUE_POLICY_BREAKDOWN=false.",
			"namespace", "type", "engine", "policy_namespace", "policy_name")
	}

	// Section 8 — rules.
	// alatyr_rule_edges: raw post-expansion count. Every rule fan-out contributes
	// exactly one edge; a single podSelector:{} manifest can produce thousands.
	// Sum-safe: sum(alatyr_rule_edges) is total edges; sum by (namespace) is
	// per-ns; sum by (action) is per-action; etc.
	// alatyr_policy_coverage: deduped per (engine, ns, name, action, coverage,
	// direction). One count per policy per bucket. sum() is NOT a valid total —
	// a policy spanning multiple buckets contributes once per bucket, so summing
	// double-counts. Ratio edges/policy_coverage per bucket = avg fan-out.
	r.ruleEdges = factory.NewGaugeVec("rule_edges",
		"Raw post-expansion rule count. One policy fans into src×dst edges; this counts every edge. Sum-safe across any label subset.",
		"namespace", "engine", "action", "coverage", "direction")
	r.policyCoverage = factory.NewGaugeVec("policy_coverage",
		"Distinct policies per (namespace, engine, action, coverage, direction) bucket. Deduped by (engine, ns, name, action, coverage, direction). sum() across labels is NOT a valid total — use alatyr_rule_edges for raw counts.",
		"namespace", "engine", "action", "coverage", "direction")
}

// promauto is a tiny local factory that MustRegisters everything on our
// registry, so registration lines read as one call per metric instead of
// three (new + register + assign). Not the upstream promauto package — we
// want registration bound to r.registry, not the default one.
type promauto struct {
	registry *prometheus.Registry
}

func (p promauto) NewGauge(name, help string) prometheus.Gauge {
	metric := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	})
	p.registry.MustRegister(metric)
	return metric
}

func (p promauto) NewGaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	}, labels)
	p.registry.MustRegister(metric)
	return metric
}

func (p promauto) NewCounter(name, help string) prometheus.Counter {
	metric := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	})
	p.registry.MustRegister(metric)
	return metric
}

func (p promauto) NewCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	metric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	}, labels)
	p.registry.MustRegister(metric)
	return metric
}

func (p promauto) NewHistogram(name, help string, buckets []float64) prometheus.Histogram {
	metric := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	})
	p.registry.MustRegister(metric)
	return metric
}

func (p promauto) NewHistogramVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	metric := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	}, labels)
	p.registry.MustRegister(metric)
	return metric
}
