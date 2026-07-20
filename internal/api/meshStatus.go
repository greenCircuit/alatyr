package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"graph/internal/models"
)

// MeshStatusResponse carries every workload's mesh membership from the cache.
// Read by UI as a separate hydration step after /api/graph so the graph
// payload stays lean. Extensible envelope — future per-workload metadata
// (traffic, scan, cost) can join the same shape without new endpoints.
type MeshStatusResponse struct {
	Nodes map[string]models.MeshMembership `json:"nodes"`
}

// MeshStatuses returns cache.MeshMembership as-is. No compute, no fetch —
// PopulateCache already wrote it during the graph build.
func (s *Server) MeshStatuses(c echo.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return c.JSON(http.StatusOK, MeshStatusResponse{Nodes: s.cache.MeshMembership})
}

