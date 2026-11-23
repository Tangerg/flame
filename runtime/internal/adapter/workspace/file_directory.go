package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

func (FileBrowser) ListDirectory(ctx context.Context, root string, plan workspaceapp.DirectoryListPlan) (_ workspaceapp.DirectoryPage, err error) {
	scope, err := resolveFileSelection(root, plan.Path)
	if err != nil {
		return workspaceapp.DirectoryPage{}, err
	}
	if !scope.directory {
		return workspaceapp.DirectoryPage{}, fmt.Errorf("%w: %q is not a directory", workspaceapp.ErrInvalidFileListPath, plan.Path)
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
	revision := directoryRevision(info)
	if plan.Revision != "" && plan.Revision != revision {
		return workspaceapp.DirectoryPage{}, workspaceapp.ErrPageCursor
	}
	scope.handle = handle
	if err := seekDirectory(ctx, directory, plan.Offset); err != nil {
		return workspaceapp.DirectoryPage{}, err
	}
	result, err := scope.readPage(ctx, directory, plan, revision)
	if err != nil {
		return workspaceapp.DirectoryPage{}, err
	}
	after, err := directory.Stat()
	if err != nil {
		return workspaceapp.DirectoryPage{}, fmt.Errorf("inspect directory revision: %w", err)
	}
	if !os.SameFile(info, after) || directoryRevision(after) != revision {
		return workspaceapp.DirectoryPage{}, workspaceapp.ErrPageCursor
	}
	return result, nil
}

func directoryRevision(info fs.FileInfo) string {
	return strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
}

const (
	minimumDirectoryScan = 256
	directoryScanPerItem = 4
)

func (scope fileSelection) readPage(ctx context.Context, directory *os.File, plan workspaceapp.DirectoryListPlan, revision string) (workspaceapp.DirectoryPage, error) {
	result := workspaceapp.DirectoryPage{Revision: revision, NextOffset: plan.Offset}
	budget := max(minimumDirectoryScan, plan.Limit*directoryScanPerItem)
	for result.ScannedEntries < budget && len(result.Entries) < plan.Limit {
		if err := context.Cause(ctx); err != nil {
			return workspaceapp.DirectoryPage{}, err
		}
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
	result.BudgetExhausted = !result.Complete && result.ScannedEntries >= budget
	return result, nil
}

func seekDirectory(ctx context.Context, directory *os.File, offset int) error {
	for remaining := offset; remaining > 0; {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		entries, err := directory.ReadDir(min(remaining, minimumDirectoryScan))
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
		case child.Name() == git.MetadataDirectory:
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
