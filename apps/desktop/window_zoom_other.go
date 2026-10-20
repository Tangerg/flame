//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// Only the macOS default zoom roles use non-reflowing magnification.
func installNativeZoom(*application.App) {}
