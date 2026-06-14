package main

import (
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"graph/internal/api"
	"graph/internal/config"
	"graph/internal/k8s"
	"graph/internal/store"
)

func main() {
	config.MustLoad(os.Getenv("CONFIG_PATH"))

	var client k8s.KubernetesClient
	if os.Getenv("DEMO_MODE") == "true" {
		demo, err := k8s.NewDemoClient(demoDataFS, "test-data")
		if err != nil {
			log.Fatalf("failed to load demo data: %v", err)
		}
		client = demo
	} else {
		real, err := k8s.New(os.Getenv("KUBECONFIG"))
		if err != nil {
			log.Fatalf("failed to create k8s client: %v", err)
		}
		client = real
	}

	e := echo.New()

	builder := store.NewBuilder(client)
	server := api.New(client, builder)
	server.RegisterRoutes(e)
	server.RegisterUI(e, uiFS)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(e.Start(":" + port))
}
