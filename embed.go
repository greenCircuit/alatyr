//go:build embed

package main

import "embed"

//go:embed ui/dist
var uiFS embed.FS

const hasUI = true
