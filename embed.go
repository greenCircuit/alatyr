package main

import "embed"

//go:embed all:ui/dist
var uiFS embed.FS

//go:embed test-data
var demoDataFS embed.FS
