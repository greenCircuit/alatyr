package main

import (
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"graph/internal/api"
	"graph/internal/k8s"
)

func main() {
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

	server := api.New(client)
	server.RegisterRoutes(e)
	if hasUI {
		server.RegisterUI(e, uiFS)
	}

	log.Fatal(e.Start(":8080"))
}
