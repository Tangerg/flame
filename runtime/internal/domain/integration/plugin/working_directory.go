package plugin

import (
	"fmt"
	"strings"
)

// The Runtime binds these variables for every package process, and a package
// may spell the same two locations as placeholders in its arguments,
// environment and working directory.
const (
	RootVariable = "PLUGIN_ROOT"
	DataVariable = "PLUGIN_DATA"

	rootPlaceholder = "${" + RootVariable + "}"
	dataPlaceholder = "${" + DataVariable + "}"
)

// ExpandPlaceholders substitutes the release and data directories a package
// value names.
func ExpandPlaceholders(value, root, data string) string {
	return strings.NewReplacer(rootPlaceholder, root, dataPlaceholder, data).Replace(value)
}

// DirectoryBase is what a package server's working directory is rooted in:
// the immutable release bytes or the installation's private data.
type DirectoryBase uint8

const (
	ReleaseBase DirectoryBase = iota + 1
	DataBase
)

// WorkingDirectory is a portable server working directory: a base and a
// resource path beneath it, empty for the base itself. It is the only reading
// of a declared directory spelling, so admission, preparation and realization
// cannot disagree about where a server runs.
type WorkingDirectory struct {
	base DirectoryBase
	path string
}

func (w WorkingDirectory) Base() DirectoryBase { return w.base }

// Path is a valid resource path beneath the base, empty for the base itself.
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

// WorkingDirectory reads where this stdio server runs. An admitted server
// always has a valid spelling; the error guards a hand-built declaration.
func (s Server) WorkingDirectory() (WorkingDirectory, error) {
	return parseWorkingDirectory(s.Dir)
}

// PackagedCommand names the release file a stdio server executes, or reports
// that its command is a bare name resolved from PATH.
func (s Server) PackagedCommand() (string, bool) {
	return strings.CutPrefix(s.Command, "./")
}
