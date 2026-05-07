package main

import (
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"graph/internal/api"
	"graph/internal/k8s"
)

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")
	client, err := k8s.New(kubeconfig)
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	e := echo.New()

	server := api.New(client)
	server.RegisterRoutes(e)
	if hasUI {
		server.RegisterUI(e, uiFS)
	}

	log.Fatal(e.Start(":8080"))
}
