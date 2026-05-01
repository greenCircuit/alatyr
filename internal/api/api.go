package api

import (
	"github.com/labstack/echo/v4"
	"main/internal/k8s"
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
