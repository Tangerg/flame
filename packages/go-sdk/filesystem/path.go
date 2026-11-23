package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const resolveOperation = "resolve"

// MaxSymlinks matches Linux MAXSYMLINKS so in-process resolution refuses
// exactly the chains the kernel would.
const MaxSymlinks = 40

func absolute(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if root == "" {
		return "", errors.New("resolve filesystem path: root is required for a relative path")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("resolve filesystem path: root must be absolute")
	}
	if filepath.VolumeName(path) != "" {
		return "", errors.New("resolve filesystem path: volume-relative paths are not supported")
	}
	if len(path) > 0 && os.IsPathSeparator(path[0]) {
		return "", errors.New("resolve filesystem path: rooted paths require an explicit volume")
	}
	// Cleaning before following symlinks changes what a subsequent .. names.
	return root + string(filepath.Separator) + path, nil
}

func ResolvePath(root, path string) (string, error) {
	abs, err := absolute(root, path)
	if err != nil {
		return "", err
	}
	resolved, err := resolve(abs, 0)
	if err != nil {
		return "", &os.PathError{Op: resolveOperation, Path: abs, Err: err}
	}
	return resolved, nil
}

// Trust boundaries must pass resolved paths so symlink aliases cannot bypass confinement.
func ContainsPath(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, fmt.Errorf("compare filesystem paths: %w", err)
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func PathsOverlap(first, second string) (bool, error) {
	if !filepath.IsAbs(first) || !filepath.IsAbs(second) {
		return false, ErrAbsolutePathRequired
	}
	if !strings.EqualFold(filepath.VolumeName(first), filepath.VolumeName(second)) {
		return false, nil
	}
	contains, err := ContainsPath(first, second)
	if err != nil || contains {
		return contains, err
	}
	return ContainsPath(second, first)
}

func resolve(abs string, depth int) (string, error) {
	if depth > MaxSymlinks {
		return "", syscall.ELOOP
	}
	abs = filepath.FromSlash(abs)
	volume := filepath.VolumeName(abs)
	separator := string(filepath.Separator)
	current := volume + separator
	info, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("inspect filesystem root %q: %w", current, err)
	}
	parts := strings.Split(abs[len(volume):], separator)
	for index, name := range parts {
		if info != nil && !info.IsDir() {
			return "", &os.PathError{Op: resolveOperation, Path: abs, Err: syscall.ENOTDIR}
		}
		switch name {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
			info = nil
			continue
		}
		candidate := filepath.Join(current, name)
		info, err = os.Lstat(candidate)
		if os.IsNotExist(err) {
			current = candidate
			info = nil
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect filesystem path %q: %w", candidate, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		target, err := symlinkTarget(candidate, current, volume)
		if err != nil {
			return "", err
		}
		// A missing component can be cancelled by .. and expose an existing
		// symlink later. Every remaining component still needs physical resolution.
		if remaining := parts[index+1:]; len(remaining) > 0 {
			target += separator + strings.Join(remaining, separator)
		}
		return resolve(target, depth+1)
	}
	return current, nil
}

func symlinkTarget(link, directory, volume string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("read symbolic link %q: %w", link, err)
	}
	target = filepath.FromSlash(target)
	switch {
	case filepath.IsAbs(target):
		return target, nil
	case filepath.VolumeName(target) != "":
		return "", fmt.Errorf("symbolic link %q has a volume-relative target", link)
	case len(target) > 0 && os.IsPathSeparator(target[0]):
		return volume + target, nil
	default:
		return directory + string(filepath.Separator) + target, nil
	}
}
