package toolset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/toolfailure"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

// Name retains Scope's absolute-input namespace. identity captures the physical
// workspace name once for shared lock/stamp keys and protected-root policy;
// neither projection can reopen or replace the retained directory authority.
type filesystemRoot struct {
	*os.Root
	identity string
}

func openFilesystemRoot(directory string) (*filesystemRoot, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("toolset: filesystem root must be absolute")
	}
	identity, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Clean(directory))
	if err != nil {
		return nil, err
	}
	opened, openedErr := root.Stat(".")
	named, namedErr := os.Stat(identity)
	if err := errors.Join(openedErr, namedErr); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	if !os.SameFile(opened, named) {
		return nil, errors.Join(errors.New("workspace changed while acquiring filesystem authority"), root.Close())
	}
	return &filesystemRoot{Root: root, identity: identity}, nil
}

// The manifest owns both handles. Every host inspection and Scope operation
// starts from the same directory authority, even if its pathname is replaced.
func openFilesystem(directory string) (*filesystemRoot, *fs.LocalExecutor, func() error, error) {
	root, err := openFilesystemRoot(directory)
	if err != nil {
		return nil, nil, nil, err
	}
	executor, err := fs.NewLocalExecutor(root.Root)
	if err != nil {
		return nil, nil, nil, errors.Join(err, root.Close())
	}
	close := sync.OnceValue(func() error { return errors.Join(executor.Close(), root.Close()) })
	return root, executor, close, nil
}

// rootRelative interprets the public Scope path vocabulary for host policy.
// os.Root and Scope still own filesystem confinement; this lexical projection
// alone is not permission to open a path.
func rootRelative(root string, path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		return "", errors.New("toolset: use an absolute or workspace-relative path instead of home shorthand")
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		path = relative
	}
	if path == "" || !filepath.IsLocal(path) {
		return "", fmt.Errorf("%w: %q", fs.ErrPathOutsideRoot, path)
	}
	return filepath.Clean(path), nil
}

// resolveRootPath discovers aliases for protected-directory policy and shared
// path locks. It never reopens root.Name(). Missing suffixes are valid mutation
// targets; links are inspected through the same root used by Scope.
func resolveRootPath(root *filesystemRoot, path string) (string, error) {
	path, err := rootRelative(root.Name(), path)
	if err != nil {
		return "", err
	}
	pending := strings.Split(path, string(filepath.Separator))
	resolved := "."
	links := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		candidate := filepath.Join(resolved, part)
		if !filepath.IsLocal(candidate) {
			return "", fs.ErrPathOutsideRoot
		}
		info, err := root.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			resolved = candidate
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			resolved = candidate
			continue
		}
		links++
		if links > 40 {
			return "", errors.New("too many symbolic links")
		}
		target, err := root.Readlink(candidate)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(target) {
			return "", fs.ErrPathOutsideRoot
		}
		pending = append(strings.Split(target, string(filepath.Separator)), pending...)
	}
	return resolved, nil
}

// Git-backed search and LSP address workspaces by pathname. They cannot use a
// detached directory handle, so never publish their observations as if they
// described this manifest's directory after its name is rebound.
func verifyWorkspacePath(root *filesystemRoot) error {
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(root.Name())
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return errors.New("workspace path changed; resolve tools again before querying path-based services")
	}
	return nil
}

func withWorkspacePath(inner toolcontract.Tool, root *filesystemRoot) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		if err := verifyWorkspacePath(root); err != nil {
			return chat.ToolOutput{}, toolfailure.Definite(err)
		}
		out, err := inner.Call(ctx, invocation)
		if err != nil {
			return out, err
		}
		if err := verifyWorkspacePath(root); err != nil {
			return chat.ToolOutput{}, toolfailure.Definite(err)
		}
		return out, nil
	})
}
