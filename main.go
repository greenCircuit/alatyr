package main

import (
	"log"

	"github.com/labstack/echo/v4"
	"main/internal/api"
	"main/internal/k8s"
)

func main() {
	client, err := k8s.New("/etc/rancher/k3s/k3s.yaml")
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	e := echo.New()

	server := api.New(client)
	server.RegisterRoutes(e)

	log.Fatal(e.Start(":8080"))
}
