package metrics

import "runtime"

// SetBuildInfo stamps alatyr_build_info=1 with the release identifiers.
// Standard prometheus pattern: the gauge value is meaningless, the labels
// carry the payload so dashboards can pin "which build produced this row".
// Empty version/commit are passed through unchanged — the metric is more
// useful with "dev"/"unknown" placeholders than with a missing series.
func (r *Recorder) SetBuildInfo(version, commit string) {
	r.buildInfo.WithLabelValues(version, commit, runtime.Version()).Set(1)
}
