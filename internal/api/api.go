package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/store"
)

type Server struct {
	client k8s.KubernetesClient
	store  *store.Builder
	cache  *models.Cache
	log    *slog.Logger
	mu     sync.RWMutex
}

func New(client k8s.KubernetesClient, store *store.Builder, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		client: client,
		store:  store,
		cache:  &models.Cache{},
		log:    logger,
	}
}

func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/graph", s.handleGraph)
	e.GET("/api/node-info", s.getNodeInfo)
	e.GET("/api/cluster-state", s.handleClusterState)
	e.GET("/api/reachable", s.getReachability)
	e.GET("/api/manifest", s.getPolicyManifest)
	e.GET("/api/issues", s.getIssues)
	e.GET("/api/mesh-status", s.MeshStatuses)
}

// RegisterUI mounts the embedded SPA at "/". Returns an error instead of
// panicking on a broken embed — main.go logs and exits so the failure is
// visible in the log stream rather than a raw runtime panic.
func (s *Server) RegisterUI(e *echo.Echo, uiFS fs.FS) error {
	distFS, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		return fmt.Errorf("mount ui/dist: %w", err)
	}
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:       ".",
		Index:      "index.html",
		HTML5:      true,
		Filesystem: http.FS(distFS),
	}))
	return nil
}
