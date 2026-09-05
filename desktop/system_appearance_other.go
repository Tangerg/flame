//go:build !darwin

package main

// Every other platform opens on the light canvas. The scheme a desktop environment
// reports is not a single value there — GTK, Qt and the freedesktop portal each answer
// separately and disagree — and guessing wrong trades one flash for another. macOS has one
// answer, so that is where this is read.
func systemPrefersDarkAppearance() bool {
	return false
}
