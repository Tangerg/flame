package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxSymlinkDepth = 40

// absolute anchors a relative path under an explicit absolute root. Host cwd
// and User Home discovery belong to process composition, never path identity.
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

// Path locks and security decisions must resolve aliases to the same resource,
// including when a suffix has not been created yet.
func ResolvePath(root, path string) (string, error) {
	abs, err := absolute(root, path)
	if err != nil {
		return "", err
	}
	resolved, err := resolve(abs, 0)
	if err != nil {
		return "", &os.PathError{Op: "resolve", Path: abs, Err: err}
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

func resolve(abs string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", errors.New("too many symbolic links")
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
			return "", &os.PathError{Op: "resolve", Path: abs, Err: syscall.ENOTDIR}
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
		target, err := os.Readlink(candidate)
		if err != nil {
			return "", fmt.Errorf("read symbolic link %q: %w", candidate, err)
		}
		target = filepath.FromSlash(target)
		if !filepath.IsAbs(target) {
			if filepath.VolumeName(target) != "" {
				return "", fmt.Errorf("symbolic link %q has a volume-relative target", candidate)
			}
			if len(target) > 0 && os.IsPathSeparator(target[0]) {
				target = volume + target
			} else {
				target = current + separator + target
			}
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
