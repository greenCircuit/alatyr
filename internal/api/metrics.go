package api

import (
	"alatyr/internal/models"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ClusterMetrics returns cluster totals + nested mesh posture counters.
// Denominators (NsTotal/WorkloadTotal) come from cache sizes at request time
// so they always match the graph the UI already holds; MeshMetrics was
// populated by BuildMeshMembership during PopulateCache.
func (s *Server) ClusterMetrics(c echo.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return c.JSON(http.StatusOK, models.ClusterMetrics{
		NsTotal:       int32(len(s.cache.NsIndex)),
		WorkloadTotal: int32(len(s.cache.WorkloadByID)),
		MeshMetrics:   s.cache.MeshMetrics,
	})
}
