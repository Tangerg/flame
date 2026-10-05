package pluginpackage

import (
	"io/fs"
	"syscall"
)

func stampOf(info fs.FileInfo) (entryStamp, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return entryStamp{}, false
	}
	return entryStamp{
		device: uint64(stat.Dev), inode: stat.Ino, mode: info.Mode(), size: info.Size(),
		modified: stat.Mtimespec.Nano(), change: stat.Ctimespec.Nano(),
	}, true
}
