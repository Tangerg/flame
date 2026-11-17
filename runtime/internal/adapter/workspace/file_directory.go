package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

// Directory pages inspect only one level. A cursor retains the directory revision
// and native enumeration offset; a changed directory requires a new traversal.
func (FileBrowser) ListDirectory(ctx context.Context, root string, plan workspaceapp.DirectoryListPlan) (_ workspaceapp.DirectoryPage, err error) {
	scope, err := resolveFileSelection(root, plan.Path)
	if err != nil {
		return workspaceapp.DirectoryPage{}, err
	}
	if !scope.directory {
		return workspaceapp.DirectoryPage{}, workspaceapp.ErrInvalidFileListPath
	}
	for _, part := range strings.Split(scope.physical, "/") {
		if part == ".git" {
			return workspaceapp.DirectoryPage{Complete: true}, nil
		}
	}
	handle, err := os.OpenRoot(scope.root)
	if err != nil {
		return workspaceapp.DirectoryPage{}, fmt.Errorf("open directory root: %w", err)
	}
	defer func() { err = errors.Join(err, handle.Close()) }()
	directory, info, err := filesystem.OpenDirectoryAt(handle, rootPath(scope.physical))
	if err != nil {
		return workspaceapp.DirectoryPage{}, fmt.Errorf("open directory page: %w", err)
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	revision := strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
	if plan.Revision != "" && plan.Revision != revision {
		return workspaceapp.DirectoryPage{}, workspaceapp.ErrPageCursor
	}
	scope.handle = handle
	for remaining := plan.Offset; remaining > 0; {
		if err := context.Cause(ctx); err != nil {
			return workspaceapp.DirectoryPage{}, err
		}
		entries, readErr := directory.ReadDir(min(remaining, 256))
		remaining -= len(entries)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return workspaceapp.DirectoryPage{}, workspaceapp.ErrPageCursor
			}
			return workspaceapp.DirectoryPage{}, fmt.Errorf("seek directory page: %w", readErr)
		}
	}
	result := workspaceapp.DirectoryPage{Revision: revision, NextOffset: plan.Offset}
	budget := max(256, plan.Limit*4)
	for result.ScannedEntries < budget && len(result.Entries) < plan.Limit {
		if err := context.Cause(ctx); err != nil {
			return workspaceapp.DirectoryPage{}, err
		}
		// Read only the remaining output capacity so every read entry is covered by this cursor.
		children, readErr := directory.ReadDir(min(plan.Limit-len(result.Entries), budget-result.ScannedEntries))
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return workspaceapp.DirectoryPage{}, fmt.Errorf("read directory page: %w", readErr)
		}
		paths := make([]string, len(children))
		for index, child := range children {
			paths[index] = path.Join(scope.physical, child.Name())
		}
		ignored := map[string]bool{}
		repository := false
		if !plan.IncludeIgnored && len(paths) > 0 {
			ignored, err = git.IgnoredPaths(ctx, scope.root, paths)
			switch {
			case err == nil:
				repository = true
			case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
			default:
				return workspaceapp.DirectoryPage{}, err
			}
		}
		for index, child := range children {
			if ignored[paths[index]] && child.IsDir() {
				tracked, trackedErr := git.HasTrackedFiles(ctx, scope.root, paths[index])
				if trackedErr != nil {
					return workspaceapp.DirectoryPage{}, trackedErr
				}
				if tracked {
					delete(ignored, paths[index])
				}
			}
			result.ScannedEntries++
			result.NextOffset++
			if child.Name() == ".git" || ignored[paths[index]] || (!repository && !plan.IncludeIgnored && child.IsDir() && backstopExclude[child.Name()]) {
				result.ExcludedEntries++
				continue
			}
			entry, exists, inspectErr := scope.inspect(path.Join(scope.logical, child.Name()))
			if inspectErr != nil {
				return workspaceapp.DirectoryPage{}, inspectErr
			}
			if exists {
				result.Entries = append(result.Entries, entry)
			} else {
				result.ExcludedEntries++
			}
		}
		if errors.Is(readErr, io.EOF) {
			result.Complete = true
			break
		}
	}
	after, err := directory.Stat()
	if err != nil {
		return workspaceapp.DirectoryPage{}, fmt.Errorf("inspect directory revision: %w", err)
	}
	if !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) || info.Size() != after.Size() {
		return workspaceapp.DirectoryPage{}, workspaceapp.ErrPageCursor
	}
	result.BudgetExhausted = !result.Complete && result.ScannedEntries >= budget
	return result, nil
}
