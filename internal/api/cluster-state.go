package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"graph/internal/graph"
)

// returning this so UI loads data without needing graph
type ClusterState struct {
	AvailableNs		[]string			`json:"availableNs"`
	StatusKeys		[]graph.StatusKey	`json:"statusKeys"`
}

func (s *Server) handleClusterState(c echo.Context) error {
	allNameSpaces, err := s.client.GetNsNames()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	data := ClusterState {
		AvailableNs: allNameSpaces,
		StatusKeys: graph.GetStatusKeys(),
	 }
	// data.availableNs = allNameSpaces
	// data.statusKeys = []graph.StatusKey{}

	return c.JSON(http.StatusOK, data)
}