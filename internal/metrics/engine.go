package metrics

import (
	"context"
	"errors"
	"time"
)

// EngineErrorReason is a closed set of buckets the engine_errors_total
// counter accepts. Free-form error strings would blow up cardinality on the
// first ephemeral network hiccup, so callers translate at the call site.
type EngineErrorReason string

const (
	EngineErrorFetch       EngineErrorReason = "fetch"
	EngineErrorDecode      EngineErrorReason = "decode"
	EngineErrorUnsupported EngineErrorReason = "unsupported"
	EngineErrorTimeout     EngineErrorReason = "timeout"
	EngineErrorUnknown     EngineErrorReason = "unknown"
)

// SetEnabledEngines stamps engine_enabled=1 for every listed engine.
// Non-listed engines are not zeroed — call SetEngineDisabled for that,
// or accept the missing series as "not compiled in".
//
// Doc note (doc/METRICS.md line 97): the gauge distinguishes "Calico absent
// from this cluster" (missing series) from "Calico broken" (present, 0).
func (r *Recorder) SetEnabledEngines(names []string) {
	for _, name := range names {
		r.engineEnabled.WithLabelValues(name).Set(1)
	}
}

// SetEngineDisabled flips engine_enabled to 0 for a name previously set to 1.
// Kept separate from SetEnabledEngines so a build with no engine hot-reload
// doesn't accidentally clear the gauge every snapshot cycle.
func (r *Recorder) SetEngineDisabled(name string) {
	r.engineEnabled.WithLabelValues(name).Set(0)
}

// RecordEngineEvaluation stamps the timing histogram + last-success timestamp
// on success, or increments engine_errors_total with a bucketed reason on
// failure. Call once per engine per snapshot cycle.
//
// reason is honoured only when err != nil; on success it's ignored so callers
// can pass EngineErrorUnknown as a placeholder.
func (r *Recorder) RecordEngineEvaluation(name string, duration time.Duration, err error, reason EngineErrorReason) {
	r.engineEvalDuration.WithLabelValues(name).Observe(duration.Seconds())
	if err != nil {
		bucket := reason
		if bucket == "" {
			bucket = classifyEngineError(err)
		}
		r.engineErrors.WithLabelValues(name, string(bucket)).Inc()
		return
	}
	r.engineLastSuccess.WithLabelValues(name).Set(float64(time.Now().Unix()))
}

// RecordEvaluation stamps the full-cycle duration histogram; on failure it
// also bumps evaluation_failures_total. evaluation_timestamp_seconds is left
// alone here — RecordSnapshot sets it, and only sets it, so age(timestamp)
// always represents "time since a snapshot was actually recorded".
func (r *Recorder) RecordEvaluation(duration time.Duration, ok bool) {
	r.evalDuration.Observe(duration.Seconds())
	if !ok {
		r.evalFailures.Inc()
	}
}

// SetInformerSynced sets the per-resource sync gauge. Call once at startup
// after the informer factory's WaitForCacheSync returns for that GVK.
func (r *Recorder) SetInformerSynced(resource string, synced bool) {
	value := 0.0
	if synced {
		value = 1
	}
	r.informerCacheSynced.WithLabelValues(resource).Set(value)
}

// classifyEngineError is a very rough fallback bucketer for callers that
// don't supply an explicit reason. Kept intentionally conservative — the
// authoritative classification is at the call site that knows what phase
// of the engine's evaluation failed.
func classifyEngineError(err error) EngineErrorReason {
	if err == nil {
		return EngineErrorUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return EngineErrorTimeout
	}
	return EngineErrorUnknown
}
