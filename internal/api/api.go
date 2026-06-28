package api

import (
	"io/fs"
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
	mu     sync.RWMutex
}

func New(client k8s.KubernetesClient, store *store.Builder) *Server {
	return &Server{
		client: client,
		store:  store,
		cache:  &models.Cache{},
	}
}

func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/graph", s.handleGraph)
	e.GET("/api/node-info", s.getNodeInfo)
	e.GET("/api/cluster-state", s.handleClusterState)
	e.GET("/api/reachable", s.getReachability)
	e.GET("/api/manifest", s.getPolicyManifest)
}

func (s *Server) RegisterUI(e *echo.Echo, uiFS fs.FS) {
	distFS, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		panic(err)
	}
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:       ".",
		Index:      "index.html",
		HTML5:      true,
		Filesystem: http.FS(distFS),
	}))
}
