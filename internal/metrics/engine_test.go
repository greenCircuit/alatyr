package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestSetEnabledEngines_StampsOneSeriesPerName covers the startup call that
// tells Grafana which engines this build has compiled in. A missing series
// downstream is the difference between "Calico not installed" and "Calico is
// silently broken" — both must be visible from /metrics.
func TestSetEnabledEngines_StampsOneSeriesPerName(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.SetEnabledEngines([]string{"k8s", "istio"})

	if got := testutil.ToFloat64(recorder.engineEnabled.WithLabelValues("k8s")); got != 1 {
		t.Errorf("engine_enabled{k8s} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.engineEnabled.WithLabelValues("istio")); got != 1 {
		t.Errorf("engine_enabled{istio} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.engineEnabled); got != 2 {
		t.Errorf("engine_enabled series count = %d, want 2 (non-listed engines must not be zeroed here)", got)
	}
}

// TestSetEngineDisabled_FlipsGaugeToZero pins the split from SetEnabledEngines.
// A build that hot-swaps an engine off must flip the gauge to 0 explicitly;
// the enable call intentionally does not clear stale series.
func TestSetEngineDisabled_FlipsGaugeToZero(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.SetEnabledEngines([]string{"k8s"})
	recorder.SetEngineDisabled("k8s")

	if got := testutil.ToFloat64(recorder.engineEnabled.WithLabelValues("k8s")); got != 0 {
		t.Errorf("engine_enabled{k8s} after disable = %v, want 0", got)
	}
	// The series must still exist — a disabled gauge at 0 is the "broken, not
	// absent" signal.
	if got := testutil.CollectAndCount(recorder.engineEnabled); got != 1 {
		t.Errorf("engine_enabled series count after disable = %d, want 1", got)
	}
}

// TestRecordEngineEvaluation_SuccessStampsTimestamp covers the happy path:
// duration observed, last-success timestamp set, no error counter incremented.
func TestRecordEngineEvaluation_SuccessStampsTimestamp(t *testing.T) {
	recorder := newTestRecorder(t)
	before := float64(time.Now().Unix())
	recorder.RecordEngineEvaluation("k8s", 500*time.Millisecond, nil, EngineErrorUnknown)

	if got := testutil.ToFloat64(recorder.engineLastSuccess.WithLabelValues("k8s")); got < before {
		t.Errorf("engine_last_success_timestamp_seconds{k8s} = %v, want >= %v", got, before)
	}
	if got := testutil.CollectAndCount(recorder.engineEvalDuration); got != 1 {
		t.Errorf("engine_evaluation_duration_seconds series = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.engineErrors); got != 0 {
		t.Errorf("engine_errors_total incremented on success: %d series, want 0", got)
	}
}

// TestRecordEngineEvaluation_ErrorBumpsBucket confirms the closed-set reason
// bucket is honoured on failure and no last-success timestamp gets stamped.
func TestRecordEngineEvaluation_ErrorBumpsBucket(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.RecordEngineEvaluation("istio", 100*time.Millisecond, errors.New("boom"), EngineErrorFetch)

	if got := testutil.ToFloat64(recorder.engineErrors.WithLabelValues("istio", "fetch")); got != 1 {
		t.Errorf("engine_errors_total{istio, fetch} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.engineLastSuccess); got != 0 {
		t.Errorf("engine_last_success stamped despite error: %d series, want 0", got)
	}
}

// TestRecordEngineEvaluation_EmptyReasonClassified covers the fallback path:
// callers passing an empty reason with a real error trigger classifyEngineError.
// Deadline exceeded must bucket to "timeout" — that is the difference between
// "cluster API is slow" and "engine is broken" on the dashboard.
func TestRecordEngineEvaluation_EmptyReasonClassified(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.RecordEngineEvaluation("k8s", 10*time.Millisecond, context.DeadlineExceeded, "")

	if got := testutil.ToFloat64(recorder.engineErrors.WithLabelValues("k8s", "timeout")); got != 1 {
		t.Errorf("engine_errors_total{k8s, timeout} = %v, want 1 (classifyEngineError fallback)", got)
	}
}

// TestClassifyEngineError_ClosedSet pins the reason vocabulary. Anything else
// silently smears the "unknown" bucket and hides an alertable pattern.
func TestClassifyEngineError_ClosedSet(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want EngineErrorReason
	}{
		{"nil", nil, EngineErrorUnknown},
		{"deadline", context.DeadlineExceeded, EngineErrorTimeout},
		{"canceled", context.Canceled, EngineErrorTimeout},
		{"wrapped deadline", errors.Join(errors.New("upstream"), context.DeadlineExceeded), EngineErrorTimeout},
		{"generic", errors.New("boom"), EngineErrorUnknown},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := classifyEngineError(testCase.err); got != testCase.want {
				t.Errorf("classifyEngineError(%v) = %q, want %q", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestRecordEvaluation_FailureBumpsCounter guards the full-cycle counter used by
// SLO alerts. RecordEvaluation must NOT stamp evaluation_timestamp_seconds —
// that gauge belongs to RecordSnapshot so age(timestamp) tracks snapshots, not
// evaluation attempts.
func TestRecordEvaluation_FailureBumpsCounter(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.RecordEvaluation(200*time.Millisecond, false)
	recorder.RecordEvaluation(100*time.Millisecond, true)

	if got := testutil.ToFloat64(recorder.evalFailures); got != 1 {
		t.Errorf("evaluation_failures_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.evalTimestamp); got != 0 {
		t.Errorf("evaluation_timestamp_seconds = %v, want 0 (RecordEvaluation must not stamp it)", got)
	}
}

// TestSetInformerSynced_BothStates covers the alertable state directly. A
// gauge stuck at 0 is one of the few signals the product uses to say "trust is
// broken" — untested until now.
func TestSetInformerSynced_BothStates(t *testing.T) {
	recorder := newTestRecorder(t)
	recorder.SetInformerSynced("networkpolicies", false)
	recorder.SetInformerSynced("networkpolicies", true)
	recorder.SetInformerSynced("authorizationpolicies", false)

	if got := testutil.ToFloat64(recorder.informerCacheSynced.WithLabelValues("networkpolicies")); got != 1 {
		t.Errorf("informer_cache_synced{networkpolicies} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.informerCacheSynced.WithLabelValues("authorizationpolicies")); got != 0 {
		t.Errorf("informer_cache_synced{authorizationpolicies} = %v, want 0", got)
	}
}
