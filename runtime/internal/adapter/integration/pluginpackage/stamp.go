package pluginpackage

import "io/fs"

// entryStamp identifies one state of a filesystem entry. It includes the
// inode change time, which every content, mode or rename change advances and
// which an unprivileged process cannot set back, unlike the modification time.
type entryStamp struct {
	device, inode    uint64
	mode             fs.FileMode
	size             int64
	modified, change int64
}
