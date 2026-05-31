package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"graph/internal/store"
)

// return all rules touching given node; lazy-populate cache for ns if missing
func (s *Server) getNodeInfo(c echo.Context) error {
	ns := c.QueryParam("namespace")
	nodeId := c.QueryParam("nodeId")
	if ns == "" || nodeId == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing params"})
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := store.GetNodeData(s.cache, nodeId, ns)
	return c.JSON(http.StatusOK, rules)
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
	result := store.IsNodesReachable(s.cache, srcId, srcNs, dstId, dstNs)
	return c.JSON(http.StatusOK, result)
}
