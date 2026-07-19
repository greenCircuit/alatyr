package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"graph/internal/models"
	"graph/internal/store"
)

// NodeDetail bundles policy-engine + mesh-source results for one workload.
// Returned by /api/node-info on node click. Cross-cutting misconfig findings
// (ambient HBONE gaps, PA hygiene) live in /api/issues, not here.
type NodeDetail struct {
	PolicyNeighbors map[string]store.NodeNeighbors    `json:"neighbors"`
	Mesh            map[string]*models.MeshMembership `json:"mesh,omitempty"`
}

// return all rules + mesh state touching given node; lazy-populate cache for
// ns if missing
func (s *Server) getNodeInfo(c echo.Context) error {
	ns := c.QueryParam("namespace")
	nodeId := c.QueryParam("nodeId")

	if ns == "" || nodeId == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing params"})
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meshSources := s.store.MeshSources()
	memberships := store.GetWorkloadMesh(c.Request().Context(), s.cache, meshSources, nodeId, ns)
	nodeNeighbors := store.BuildNodeNeighbor(s.cache, nodeId)

	detail := NodeDetail{
		PolicyNeighbors: nodeNeighbors,
		Mesh:            memberships,
	}
	return c.JSON(http.StatusOK, detail)
}

// getReachability returns the structured per-engine verdict for src→dst.
// Frontend renders a panel breaking down lock state, matched allow/deny rules,
// and the engine that blocked the path when verdict is "deny".
func (s *Server) getReachability(c echo.Context) error {
	srcId := c.QueryParam("srcId")
	srcNs := c.QueryParam("srcNs")
	dstId := c.QueryParam("dstId")
	dstNs := c.QueryParam("dstNs")
	if srcId == "" || srcNs == "" || dstId == "" || dstNs == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing params (srcId, srcNs, dstId, dstNs)"})
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	result := store.IsEndpointsReachable(c.Request().Context(), s.cache, s.store.MeshSource(), srcId, srcNs, dstId, dstNs)
	return c.JSON(http.StatusOK, result)
}

// getIssues returns every cross-cutting policy/mesh conflict found across the
// whole cached graph. Whole-cluster scan, not node-scoped.
func (s *Server) getIssues(c echo.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	issues := store.GetIssues(c.Request().Context(), s.cache, s.store.MeshSource())
	return c.JSON(http.StatusOK, issues)
}
