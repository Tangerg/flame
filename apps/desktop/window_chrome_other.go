//go:build !darwin

package main

import "unsafe"

// Only macOS puts window controls over the app's own content; elsewhere nothing
// needs to be reserved.

func nativeWindowChrome(unsafe.Pointer) (controlsCentreY, controlsInlineEnd float64, measured bool) {
	return 0, 0, false
}
