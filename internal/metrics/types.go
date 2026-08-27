package metrics

// policyCoverageKey is the label tuple emitted by alatyr_policy_coverage.
// Must match the metric's label set 1:1 — adding a label here requires
// adding it to WithLabelValues at the write site and to the dedup key
// in recordRulMetrics, or dedup silently collapses buckets.
type policyCoverageKey struct {
	namespace string
	engine    string
	action    string
	coverage  string
	direction string
}
