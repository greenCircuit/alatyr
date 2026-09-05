package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"alatyr/internal/k8s"
	"alatyr/internal/metrics"
	"alatyr/internal/models"
	"alatyr/internal/store"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	client  k8s.KubernetesClient
	store   *store.Builder
	cache   *models.Cache
	log     *slog.Logger
	metrics *metrics.Recorder // nil when metrics are disabled — every call site nil-checks
	mu      sync.RWMutex
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

// SetMetrics wires the Prometheus recorder. Optional — the server runs
// without it — but cache-refresh and the /metrics endpoint both need it,
// so main is expected to call this before RegisterRoutes.
func (s *Server) SetMetrics(recorder *metrics.Recorder) {
	s.metrics = recorder
}

func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/graph", s.handleGraph)
	e.GET("/api/node-info", s.getNodeInfo)
	e.GET("/api/cluster-state", s.handleClusterState)
	e.GET("/api/reachable", s.getReachability)
	e.GET("/api/manifest", s.getPolicyManifest)
	e.GET("/api/issues", s.getIssues)
	e.GET("/api/mesh-status", s.MeshStatuses)
	e.GET("/api/cluster-metrics", s.ClusterMetrics)
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
