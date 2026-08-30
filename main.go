package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"graph/internal/api"
	"graph/internal/config"
	"graph/internal/k8s"
	"graph/internal/logging"
	"graph/internal/store"
)

func main() {
	manifestDir := flag.String("f", "", "load manifests from this directory tree instead of a live cluster")
	flag.Parse()

	logger := logging.New(logging.LevelFromEnv())
	slog.SetDefault(logger)

	cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
	if err != nil {
		logger.Error("config load failed",
			slog.String("phase", "startup"),
			slog.String("path", os.Getenv("CONFIG_PATH")),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	config.Install(cfg)

	var client k8s.KubernetesClient
	switch {
	case *manifestDir != "":
		demo, err := k8s.NewDemoClient(os.DirFS(*manifestDir), ".")
		if err != nil {
			logger.Error("manifest dir load failed",
				slog.String("phase", "startup"),
				slog.String("dir", *manifestDir),
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		client = demo
	case os.Getenv("DEMO_MODE") == "true":
		demo, err := k8s.NewDemoClient(demoDataFS, "test-data")
		if err != nil {
			logger.Error("demo data load failed",
				slog.String("phase", "startup"),
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		client = demo
	default:
		stopCh := make(chan struct{})
		defer close(stopCh)
		real, err := k8s.NewInformerClient(os.Getenv("KUBECONFIG"), stopCh)
		if err != nil {
			logger.Error("k8s informer client init failed",
				slog.String("phase", "startup"),
				slog.String("kubeconfig", os.Getenv("KUBECONFIG")),
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		client = real
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(logging.Middleware(logger))

	builder := store.NewBuilder(client, logger)
	server := api.New(client, builder, logger)
	server.RegisterRoutes(e)
	if err := server.RegisterUI(e, uiFS); err != nil {
		logger.Error("register UI failed",
			slog.String("phase", "startup"),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	logger.Info("http server starting",
		slog.String("phase", "startup"),
		slog.String("port", port),
	)
	if err := e.Start(":" + port); err != nil {
		logger.Error("http server exited",
			slog.String("phase", "runtime"),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
}
