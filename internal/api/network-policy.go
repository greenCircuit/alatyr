package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"graph/internal/graph"
)

// return full graph
func (s *Server) handleGraph(c echo.Context) error {
	var namespaces []string
	if nsParam := c.QueryParam("namespaces"); nsParam != "" {
		namespaces = strings.Split(nsParam, ",")
	} else {
		var err error
		namespaces, err = s.client.GetNsNames()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	s.mu.RLock()
	cache := s.cache
	s.mu.RUnlock()
	g := graph.BuildGraph(cache, namespaces)

	return c.JSON(http.StatusOK, g)
}
