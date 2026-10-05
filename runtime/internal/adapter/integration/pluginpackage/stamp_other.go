//go:build !linux && !darwin

package pluginpackage

import "io/fs"

// Other platforms expose no change time the Runtime user cannot set, so every
// launch rescans the release in full.
func stampOf(fs.FileInfo) (entryStamp, bool) { return entryStamp{}, false }
