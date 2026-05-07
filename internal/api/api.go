package api

import (
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"graph/internal/k8s"
)

type Server struct {
	client *k8s.Client
}

func New(client *k8s.Client) *Server {
	return &Server{client: client}
}

func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/graph", s.handleGraph)
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
