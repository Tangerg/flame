package plugin

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
)

const (
	rootPlaceholder = "${" + process.RootVariable + "}"
	dataPlaceholder = "${" + process.DataVariable + "}"
)

func ExpandPlaceholders(value, root, data string) string {
	return strings.NewReplacer(rootPlaceholder, root, dataPlaceholder, data).Replace(value)
}

type DirectoryBase uint8

const (
	ReleaseBase DirectoryBase = iota + 1
	DataBase
)

type WorkingDirectory struct {
	base DirectoryBase
	path string
}

func (w WorkingDirectory) Base() DirectoryBase { return w.base }

func (w WorkingDirectory) Path() string { return w.path }

func parseWorkingDirectory(dir string) (WorkingDirectory, error) {
	switch dir {
	case "", rootPlaceholder:
		return WorkingDirectory{base: ReleaseBase}, nil
	case dataPlaceholder:
		return WorkingDirectory{base: DataBase}, nil
	}
	var result WorkingDirectory
	switch {
	case strings.HasPrefix(dir, "./"):
		result = WorkingDirectory{base: ReleaseBase, path: dir[2:]}
	case strings.HasPrefix(dir, rootPlaceholder+"/"):
		result = WorkingDirectory{base: ReleaseBase, path: strings.TrimPrefix(dir, rootPlaceholder+"/")}
	case strings.HasPrefix(dir, dataPlaceholder+"/"):
		result = WorkingDirectory{base: DataBase, path: strings.TrimPrefix(dir, dataPlaceholder+"/")}
	default:
		return WorkingDirectory{}, fmt.Errorf("%w: package working directory", ErrInvalid)
	}
	if !ValidResourcePath(result.path) {
		return WorkingDirectory{}, fmt.Errorf("%w: package working directory path", ErrInvalid)
	}
	return result, nil
}

func (s Server) WorkingDirectory() (WorkingDirectory, error) {
	stdio := s.stdio()
	return parseWorkingDirectory(stdio.Dir)
}

func (s Server) PackagedCommand() (string, bool) {
	stdio := s.stdio()
	return strings.CutPrefix(stdio.Command, "./")
}
