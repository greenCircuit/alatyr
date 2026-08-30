package main

import "embed"

//go:embed all:ui/dist
var uiFS embed.FS

// Only the full demo cluster is embedded. test-data/scenarios/* are small
// single-purpose clusters loaded from disk with -f, not shipped in the binary.
//go:embed test-data/demo
var demoDataFS embed.FS
