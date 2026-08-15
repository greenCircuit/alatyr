package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"graph/internal/api"
	"graph/internal/config"
	"graph/internal/k8s"
	"graph/internal/logging"
	"graph/internal/metrics"
	"graph/internal/store"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// buildVersion + buildCommit are injected at link time via -ldflags -X.
// Defaults let a plain `go run` still stamp alatyr_build_info with something
// recognizable rather than an empty label.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
)

func main() {
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

	stopCh := make(chan struct{})
	defer close(stopCh)

	var client k8s.KubernetesClient
	// informerClient is the concrete type when the real k8s path is taken —
	// kept alongside `client` so SyncedResources can be surfaced to metrics
	// without leaking a k8s type through the KubernetesClient interface.
	var informerClient *k8s.InformerClient
	if os.Getenv("DEMO_MODE") == "true" {
		demo, err := k8s.NewDemoClient(demoDataFS, "test-data")
		if err != nil {
			logger.Error("demo data load failed",
				slog.String("phase", "startup"),
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		client = demo
	} else {
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
		informerClient = real
	}

	metricsRecorder := metrics.New(metrics.Config{
		Detail:               metrics.DetailLevel(os.Getenv("METRICS_DETAIL")),
		IssuePolicyBreakdown: parseIssuePolicyBreakdown(os.Getenv("METRICS_ISSUE_POLICY_BREAKDOWN")),
	}, logger)
	metricsRecorder.SetBuildInfo(buildVersion, buildCommit)
	if informerClient != nil {
		for _, resource := range informerClient.SyncedResources() {
			metricsRecorder.SetInformerSynced(resource, true)
		}
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	// HTTP metrics middleware before logging so latency captures the full
	// request lifecycle. Order matches Echo's outside-in invocation.
	e.Use(metricsRecorder.Middleware())
	e.Use(logging.Middleware(logger))

	builder := store.NewBuilder(client, logger)
	metricsRecorder.SetEnabledEngines(builder.EngineNames())

	server := api.New(client, builder, logger)
	server.SetMetrics(metricsRecorder)

	refresh := time.Duration(cfg.CacheRefreshSec) * time.Second
	if refresh <= 0 {
		refresh = 10 * time.Second
	}
	if err := server.RefreshAll(); err != nil {
		logger.Warn("initial cache prime failed",
			slog.String("phase", "startup"),
			slog.String("error", err.Error()),
		)
	}
	go server.RunCacheRefresh(refresh, stopCh)

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

	// Separate metrics listener. Kept off the main Echo so scrape traffic
	// stays clear of Recover + logging + HTTP-metrics middleware — the
	// middleware would otherwise record /metrics as its own busiest route
	// and create feedback-loop cardinality on the dashboard. Started in a
	// goroutine because e.Start below blocks the main goroutine.
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8085"
	}

	metricsEcho := echo.New()
	metricsEcho.HideBanner = true
	metricsEcho.HidePort = true
	metricsEcho.GET("/metrics", echo.WrapHandler(metricsRecorder.Handler()))
	metricsEcho.Server.ReadHeaderTimeout = 5 * time.Second

	go func() {
		logger.Info("metrics server starting",
			slog.String("phase", "startup"),
			slog.String("port", metricsPort),
		)
		if err := metricsEcho.Start(":" + metricsPort); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server exited",
				slog.String("phase", "runtime"),
				slog.String("error", err.Error()),
			)
		}
	}()

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

// parseIssuePolicyBreakdown reads the METRICS_ISSUE_POLICY_BREAKDOWN env var.
// Empty string returns nil so the Recorder keeps its default-on behavior.
// Explicit "false"/"0"/"off"/"no" disables; anything else enables.
func parseIssuePolicyBreakdown(raw string) *bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return nil
	}
	enabled := true
	switch value {
	case "false", "0", "off", "no":
		enabled = false
	}
	return &enabled
}
