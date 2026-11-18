package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"

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
	if insideGitMetadata(scope.physical) {
		return workspaceapp.DirectoryPage{Complete: true}, nil
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
	if err := seekDirectory(ctx, directory, plan.Offset); err != nil {
		return workspaceapp.DirectoryPage{}, err
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
		if err := scope.page(ctx, children, plan.IncludeIgnored, &result); err != nil {
			return workspaceapp.DirectoryPage{}, err
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

// seekDirectory skips the entries an earlier page already covered. A directory
// that ends before the offset no longer matches the cursor.
func seekDirectory(ctx context.Context, directory *os.File, offset int) error {
	for remaining := offset; remaining > 0; {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		entries, err := directory.ReadDir(min(remaining, 256))
		remaining -= len(entries)
		if errors.Is(err, io.EOF) {
			return workspaceapp.ErrPageCursor
		}
		if err != nil {
			return fmt.Errorf("seek directory page: %w", err)
		}
	}
	return nil
}

// page records one read of children: every child advances the cursor, and
// only a presentable, unexcluded child becomes an entry.
func (s fileSelection) page(ctx context.Context, children []os.DirEntry, includeIgnored bool, result *workspaceapp.DirectoryPage) error {
	excluded, err := s.excludedChildren(ctx, children, includeIgnored)
	if err != nil {
		return err
	}
	for index, child := range children {
		result.ScannedEntries++
		result.NextOffset++
		if excluded[index] {
			result.ExcludedEntries++
			continue
		}
		entry, exists, err := s.inspect(path.Join(s.logical, child.Name()))
		if err != nil {
			return err
		}
		if !exists {
			result.ExcludedEntries++
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	return nil
}

// excludedChildren applies Git ignore rules, keeping an ignored directory that
// still holds tracked files. Outside a repository the backstop excludes bulky
// directories instead.
func (s fileSelection) excludedChildren(ctx context.Context, children []os.DirEntry, includeIgnored bool) ([]bool, error) {
	paths := make([]string, len(children))
	for index, child := range children {
		paths[index] = path.Join(s.physical, child.Name())
	}
	ignored := map[string]bool{}
	repository := false
	if !includeIgnored && len(paths) > 0 {
		var err error
		ignored, err = git.IgnoredPaths(ctx, s.root, paths)
		switch {
		case err == nil:
			repository = true
		case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
		default:
			return nil, err
		}
	}
	excluded := make([]bool, len(children))
	for index, child := range children {
		switch {
		case child.Name() == gitMetadataDir:
			excluded[index] = true
		case ignored[paths[index]] && child.IsDir():
			tracked, err := git.HasTrackedFiles(ctx, s.root, paths[index])
			if err != nil {
				return nil, err
			}
			excluded[index] = !tracked
		case ignored[paths[index]]:
			excluded[index] = true
		default:
			excluded[index] = !repository && !includeIgnored && child.IsDir() && backstopExclude[child.Name()]
		}
	}
	return excluded, nil
}
