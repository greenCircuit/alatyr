package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"main/internal/graph"
)

func (s *Server) handleGraph(c echo.Context) error {
	nsParam := c.QueryParam("namespaces")
	if nsParam == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "namespaces query param required"})
	}

	namespaces := strings.Split(nsParam, ",")

	g, err := graph.BuildGraph(namespaces, s.client)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, g)
}
