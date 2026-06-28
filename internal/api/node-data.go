package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"graph/internal/models"
	"graph/internal/store"
	meshistio "graph/internal/mesh/istio"
)

// NodeDetail bundles policy-engine + mesh-source results + interop issues
// for one workload. Returned by /api/node-info on node click. Issues are
// cross-cutting misconfig findings that don't belong inside any single
// policy or mesh entry (e.g. ambient pod with NP missing ztunnel allowance).
type NodeDetail struct {
	Policies map[string]models.NodeInfo        `json:"policies"`
	Mesh     map[string]*models.MeshMembership `json:"mesh,omitempty"`
	Issues   []string                          `json:"issues,omitempty"`
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
	policies:= store.GetNodeData(s.cache, nodeId, ns) 
	var meshIssues []string
	// check for istio issues if part of istio ambient mode
	_, ok := memberships[meshistio.SourceName] 
	if ok {
		istioAmbientIssues := meshistio.ValidateExternalRules(s.cache, memberships[meshistio.SourceName], policies)
		meshIssues = append(meshIssues, istioAmbientIssues...)
	}

	detail := NodeDetail{
		Policies: policies,
		Mesh:     memberships,
		Issues:   meshIssues,
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
	result := store.IsNodesReachable(c.Request().Context(), s.cache, s.store.MeshSources(), srcId, srcNs, dstId, dstNs)
	return c.JSON(http.StatusOK, result)
}
