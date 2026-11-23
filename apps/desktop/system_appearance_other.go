//go:build !darwin

package main

// Other desktops have no single answer (GTK, Qt and the freedesktop portal
// disagree), and guessing wrong trades one flash for another.
func systemPrefersDarkAppearance() bool {
	return false
}
