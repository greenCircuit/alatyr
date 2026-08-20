package metrics

// MetricsNamespace is the shared Prometheus namespace ("alatyr_" prefix) every
// metric emitted from this package uses. Renaming it silently breaks every
// downstream dashboard and alert — treat as append-only.
const MetricsNamespace = "alatyr"

// ClusterScopeNamespace is the sentinel stamped into the `namespace` (or
// `policy_namespace`) label whenever a policy object is cluster-scoped and
// therefore has no real namespace — e.g. Calico GlobalNetworkPolicy. Kept
// starting with an underscore because valid k8s namespace names are DNS-1123
// labels (no leading underscore), so this can never collide with a real ns.
// Making it explicit is not cosmetic: the tool exists to surface cluster-scoped
// policy, so it must not render as a null on its own dashboard.
const ClusterScopeNamespace = "_cluster"

// DetailLevel gates high-cardinality metric families. Off by default because
// per-workload gauges scale as pods × statuses, which is exactly the failure
// mode the metrics doc's cardinality section warns about.
type DetailLevel string

const (
	DetailDefault  DetailLevel = ""         // namespace + one dimension key only
	DetailWorkload DetailLevel = "workload" // adds workload-labelled series
)

// Config controls what the Recorder registers. Populate at process start;
// changes at runtime are not supported (register-once is a client_golang
// constraint, not one we impose).
//
// IssuePolicyBreakdown controls registration of alatyr_issues_by_policy.
// Nil means enabled (default). Set to a pointer to false to disable when
// the culprit fan-out (issues × culprits-per-issue) becomes prohibitive.
type Config struct {
	Detail               DetailLevel
	IssuePolicyBreakdown *bool
}

// IssuePolicyBreakdownEnabled reports whether the per-culprit issue
// breakdown should be registered. Nil pointer keeps the default of on.
func (c Config) IssuePolicyBreakdownEnabled() bool {
	if c.IssuePolicyBreakdown == nil {
		return true
	}
	return *c.IssuePolicyBreakdown
}
