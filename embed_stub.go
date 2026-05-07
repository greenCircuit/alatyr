//go:build !embed

package main

import "embed"

var uiFS embed.FS

const hasUI = false
