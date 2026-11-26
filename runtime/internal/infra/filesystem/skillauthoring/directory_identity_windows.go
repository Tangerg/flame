package skillauthoring

import (
	"os"

	"golang.org/x/sys/windows"
)

func directoryIdentity(directory *os.File) (volume, index uint64, err error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(directory.Fd()), &info); err != nil {
		return 0, 0, err
	}
	return uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), nil
}
