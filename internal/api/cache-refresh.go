package api

import (
	"context"
	"log/slog"
	"time"

	"alatyr/internal/metrics"
	"alatyr/internal/models"
	"alatyr/internal/store"
)

// RefreshAll runs one full snapshot: fetch every namespace, evaluate every
// engine, resolve mesh membership, then swap the finished cache into the
// server under the write lock. On success the new cache is immutable —
// downstream readers get a stable pointer without locking.
//
// Metrics are recorded on the tail of both success and failure paths so
// evaluation duration, per-engine timings, and error counters keep flowing
// even when a cycle bails out partway. alatyr_evaluation_timestamp_seconds
// is deliberately only advanced on success — dashboards use its age as the
// "is anything happening?" signal, so a failed cycle must not touch it.
func (s *Server) RefreshAll() error {
	nss, err := s.client.GetNsNames()
	if err != nil {
		s.recordEvaluation(store.PopulateReport{}, false)
		return err
	}
	newCache := &models.Cache{}
	report, populateErr := s.store.PopulateCache(newCache, nss)
	if populateErr != nil {
		s.recordEvaluation(report, false)
		return populateErr
	}
	s.mu.Lock()
	s.cache = newCache
	s.mu.Unlock()
	s.recordEvaluation(report, true)
	s.recordSnapshot(newCache)
	return nil
}

func (s *Server) RunCacheRefresh(interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := s.RefreshAll(); err != nil {
				s.log.Warn("cache refresh failed",
					slog.String("phase", "cache_refresh"),
					slog.String("error", err.Error()))
			}
		}
	}
}

// recordEvaluation feeds the per-cycle observability counters. Split from
// RefreshAll so the failure branches don't grow a body of nil-guards; the
// method itself is the nil-guard.
func (s *Server) recordEvaluation(report store.PopulateReport, ok bool) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordEvaluation(report.Duration, ok)
	for _, engineReport := range report.Engines {
		s.metrics.RecordEngineEvaluation(engineReport.Name, engineReport.Duration, engineReport.Err, metrics.EngineErrorUnknown)
	}
}

// recordSnapshot rebuilds every snapshot-derived gauge from the freshly
// swapped cache. Issues are computed once here rather than lazily by the
// /issues handler because the metrics endpoint has no request scope to
// trigger evaluation on scrape. Uses context.Background — the snapshot loop
// isn't tied to any inbound request.
func (s *Server) recordSnapshot(cache *models.Cache) {
	if s.metrics == nil {
		return
	}
	issues := store.GetIssues(context.Background(), cache, s.store.MeshSource())
	s.metrics.RecordSnapshot(cache, issues, s.store.EngineNames())
}
