package api

import (
	"net/http"

	"alatyr/internal/models"
	"alatyr/internal/policy"

	"github.com/labstack/echo/v4"
)

// get what ns, polices are available on cluster without needing it graph object for it.
// Allows viewing this right away without waiting for graph to come up
type ClusterState struct {
	AvailableNs   []string           `json:"availableNs"`
	StatusKeys    []models.StatusKey `json:"statusKeys"`
	PolicySources []string           `json:"policySources"`
}

func (s *Server) handleClusterState(c echo.Context) error {
	allNameSpaces, err := s.client.GetNsNames()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	data := ClusterState{
		AvailableNs:   allNameSpaces,
		StatusKeys:    policy.AllStatusKeys(),
		PolicySources: s.store.EngineNames(),
	}

	return c.JSON(http.StatusOK, data)
}
