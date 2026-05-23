package api

import (
	"net/http"

	"graph/internal/models"
	"graph/internal/policy"

	"github.com/labstack/echo/v4"
)

// returning this so UI loads data without needing graph
type ClusterState struct {
	AvailableNs []string           `json:"availableNs"`
	StatusKeys  []models.StatusKey `json:"statusKeys"`
}

func (s *Server) handleClusterState(c echo.Context) error {
	allNameSpaces, err := s.client.GetNsNames()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	data := ClusterState{
		AvailableNs: allNameSpaces,
		StatusKeys:  policy.AllStatusKeys(),
	}

	return c.JSON(http.StatusOK, data)
}
