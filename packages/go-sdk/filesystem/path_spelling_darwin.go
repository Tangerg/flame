package filesystem

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func directorySpelling(path string) (string, error) {
	name, err := unix.BytePtrFromString(path)
	if err != nil {
		return "", err
	}
	attributes := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_FULLPATH}
	var result struct {
		size      uint32
		reference struct {
			offset int32
			length uint32
		}
		data [unix.PathMax]byte
	}
	// The pathname attribute preserves the spelling without requiring directory read access.
	_, _, errno := syscall.Syscall6(unix.SYS_GETATTRLIST,
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&result)), unsafe.Sizeof(result), 0, 0)
	if errno != 0 {
		return "", errno
	}
	// The kernel measures the variable-length result from the reference itself and can truncate it.
	start := int(result.reference.offset) - int(unsafe.Sizeof(result.reference))
	length := int(result.reference.length)
	if start < 0 || start > len(result.data) || length == 0 || length > len(result.data)-start {
		return "", errors.New("filesystem: invalid macOS directory path attribute")
	}
	return unix.ByteSliceToString(result.data[start : start+length]), nil
}
