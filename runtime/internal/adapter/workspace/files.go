package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

const maxListEntries = 20000

func insideGitMetadata(physical string) bool {
	return slices.Contains(strings.Split(physical, "/"), git.MetadataDirectory)
}

var backstopExclude = map[string]bool{
	git.MetadataDirectory: true, "node_modules": true, ".next": true, "dist": true,
	"build": true, "target": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, ".idea": true, ".vscode": true,
	".cache": true, "coverage": true, ".turbo": true, ".svn": true, ".hg": true,
}

func SearchFiles(ctx context.Context, root, selected, glob string) ([]workspaceapp.FileEntry, error) {
	scope, err := resolveFileSelection(root, selected)
	if err != nil {
		return nil, err
	}
	return scope.list(ctx, workspaceapp.FileListOptions{Recursive: true, Glob: glob})
}

func (scope fileSelection) list(ctx context.Context, opts workspaceapp.FileListOptions) (_ []workspaceapp.FileEntry, err error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	rootHandle, err := os.OpenRoot(scope.root)
	if err != nil {
		return nil, fmt.Errorf("open listing root %q: %w", scope.root, err)
	}
	defer func() {
		err = errors.Join(err, rootHandle.Close())
	}()
	scope.handle = rootHandle
	glob, err := compileGlob(opts.Glob)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", workspaceapp.ErrInvalidFileGlob, err)
	}
	if insideGitMetadata(scope.physical) {
		return []workspaceapp.FileEntry{}, nil
	}
	files, tracked, err := scope.trackedFiles(ctx, opts.IncludeIgnored)
	if err != nil {
		return nil, err
	}
	if !tracked {
		return scope.listUntracked(ctx, opts, glob)
	}
	return scope.entries(ctx, files, opts, glob)
}

func (scope fileSelection) trackedFiles(ctx context.Context, includeIgnored bool) ([]string, bool, error) {
	if includeIgnored {
		return nil, false, nil
	}
	files, err := git.ListFiles(ctx, scope.root, scope.physical, maxListEntries)
	switch {
	case err == nil:
		return scope.project(files), true, nil
	case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
		return nil, false, nil
	case errors.Is(err, git.ErrResultTooLarge):
		return nil, false, fmt.Errorf("%w: %w", workspaceapp.ErrFileListTooLarge, err)
	default:
		return nil, false, err
	}
}

func (scope fileSelection) listUntracked(ctx context.Context, opts workspaceapp.FileListOptions, glob fileGlob) ([]workspaceapp.FileEntry, error) {
	if !opts.IncludeIgnored && scope.backstopExcluded() {
		return []workspaceapp.FileEntry{}, nil
	}
	// Walking the whole subtree first would defeat lazy loading: a home-directory
	// workspace could hit an unreadable descendant or the safety limit before
	// returning its immediate children.
	if !opts.Recursive && opts.Glob == "" {
		return levelFilesystemEntries(ctx, scope, opts.IncludeIgnored)
	}
	files := []string{scope.logical}
	if scope.directory {
		walked, err := walkFiles(ctx, scope, opts.IncludeIgnored, maxListEntries)
		if err != nil {
			return nil, err
		}
		files = walked
	}
	return scope.entries(ctx, files, opts, glob)
}

func (scope fileSelection) backstopExcluded() bool {
	directory := scope.physical
	if !scope.directory {
		directory = path.Dir(directory)
	}
	for _, part := range strings.Split(directory, "/") {
		if backstopExclude[part] {
			return true
		}
	}
	return false
}

func (scope fileSelection) entries(ctx context.Context, files []string, opts workspaceapp.FileListOptions, glob fileGlob) ([]workspaceapp.FileEntry, error) {
	if len(files) > maxListEntries {
		return nil, fmt.Errorf("%w: more than %d files", workspaceapp.ErrFileListTooLarge, maxListEntries)
	}
	if opts.Recursive || opts.Glob != "" {
		return recursiveFiles(ctx, scope, files, glob)
	}
	return levelEntries(scope, files)
}

type fileSelection struct {
	root      string
	logical   string
	physical  string
	directory bool
	handle    *os.Root
}

func resolveFileSelection(root, sub string) (fileSelection, error) {
	sub = path.Clean(filepath.ToSlash(sub))
	if sub == "." {
		sub = ""
	}
	if path.IsAbs(sub) {
		return fileSelection{}, fmt.Errorf("%w: %q is not relative to the workspace", workspaceapp.ErrInvalidFileListPath, sub)
	}
	physicalRoot, err := filesystem.ResolvePath("", root)
	if err != nil {
		return fileSelection{}, fmt.Errorf("inspect listing root %q: %w", root, err)
	}
	physicalPath, err := filesystem.ResolvePath(physicalRoot, filepath.FromSlash(sub))
	if err != nil {
		return fileSelection{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	inside, err := filesystem.ContainsPath(physicalRoot, physicalPath)
	if err != nil {
		return fileSelection{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if !inside {
		return fileSelection{}, fmt.Errorf("%w: %q escapes the workspace", workspaceapp.ErrInvalidFileListPath, sub)
	}
	info, err := os.Stat(physicalPath)
	if errors.Is(err, os.ErrNotExist) {
		return fileSelection{}, fmt.Errorf("%w: %q does not exist", workspaceapp.ErrInvalidFileListPath, sub)
	}
	if err != nil {
		return fileSelection{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fileSelection{}, fmt.Errorf("%w: %q is not a regular file or directory", workspaceapp.ErrInvalidFileListPath, sub)
	}
	physical, err := filepath.Rel(physicalRoot, physicalPath)
	if err != nil {
		return fileSelection{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if physical == "." {
		physical = ""
	}
	return fileSelection{
		root: physicalRoot, logical: sub, physical: filepath.ToSlash(physical), directory: info.IsDir(),
	}, nil
}

func (scope fileSelection) project(files []string) []string {
	if !scope.directory {
		if slices.Contains(files, scope.physical) {
			return []string{scope.logical}
		}
		return nil
	}
	if scope.logical == scope.physical {
		return files
	}
	projected := make([]string, 0, len(files))
	for _, file := range files {
		relative, ok := listingRelativePath(file, scope.physical)
		if !ok {
			continue
		}
		projected = append(projected, path.Join(scope.logical, relative))
	}
	return projected
}

func (scope fileSelection) inspect(logical string) (workspaceapp.FileEntry, bool, error) {
	physical := logical
	if scope.logical != scope.physical {
		if !scope.directory && logical == scope.logical {
			physical = scope.physical
		} else {
			relative, ok := listingRelativePath(logical, scope.logical)
			if !ok {
				return workspaceapp.FileEntry{}, false, nil
			}
			physical = path.Join(scope.physical, relative)
		}
	}
	entry, exists, err := inspectEntry(scope.handle, physical)
	if err != nil || !exists {
		return entry, exists, err
	}
	entry.Path = logical
	entry.Name = path.Base(logical)
	return entry, true, nil
}

func levelFilesystemEntries(ctx context.Context, scope fileSelection, includeIgnored bool) ([]workspaceapp.FileEntry, error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	children, err := readDirectoryEntries(scope.handle, scope.physical, maxListEntries)
	if err != nil {
		return nil, err
	}
	entries := make([]workspaceapp.FileEntry, 0, len(children))
	for _, child := range children {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return nil, err
		}
		if child.Name() == git.MetadataDirectory || (child.IsDir() && !includeIgnored && backstopExclude[child.Name()]) {
			continue
		}
		rel := path.Join(scope.logical, child.Name())
		entry, exists, err := scope.inspect(rel)
		if err != nil {
			return nil, err
		}
		if exists {
			entries = append(entries, entry)
		}
	}
	sortFileEntries(entries)
	return entries, nil
}

func readDirectoryEntries(root *os.Root, directory string, limit int) (_ []fs.DirEntry, err error) {
	if limit < 0 {
		return nil, errors.New("workspace: directory entry limit must not be negative")
	}
	name := rootPath(directory)
	dir, _, err := filesystem.OpenDirectoryAt(root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, filesystem.ErrNotDirectory) || errors.Is(err, filesystem.ErrChanged) {
			return nil, fmt.Errorf("%w: %q: %w", workspaceapp.ErrInvalidFileListPath, directory, err)
		}
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close listing directory %q: %w", directory, closeErr))
		}
	}()

	// Read one sentinel entry beyond the contract limit. os.ReadDir(directory)
	// would materialize an unbounded directory before the safety policy had a
	// chance to reject it.
	children, err := dir.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	if len(children) > limit {
		return nil, fmt.Errorf("%w: more than %d entries in %q", workspaceapp.ErrFileListTooLarge, limit, directory)
	}
	slices.SortFunc(children, func(left, right fs.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	return children, nil
}

func walkFiles(ctx context.Context, scope fileSelection, includeIgnored bool, maximumEntries int) ([]string, error) {
	var files []string
	pendingDirectories := []string{scope.physical}
	visitedEntries := 0
	for len(pendingDirectories) > 0 {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return nil, err
		}
		directory := pendingDirectories[0]
		pendingDirectories[0] = ""
		pendingDirectories = pendingDirectories[1:]
		children, err := readDirectoryEntries(scope.handle, directory, maximumEntries-visitedEntries)
		if err != nil {
			return nil, fmt.Errorf("walk %q: %w", scope.physical, err)
		}
		visitedEntries += len(children)
		for _, child := range children {
			if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
				return nil, err
			}
			if child.Name() == git.MetadataDirectory {
				continue
			}
			candidate := path.Join(directory, child.Name())
			if child.IsDir() {
				if !includeIgnored && backstopExclude[child.Name()] {
					continue
				}
				pendingDirectories = append(pendingDirectories, candidate)
				continue
			}
			relative, inside := listingRelativePath(candidate, scope.physical)
			if !inside {
				return nil, fmt.Errorf("walk path %q is outside root %q", candidate, scope.physical)
			}
			files = append(files, path.Join(scope.logical, relative))
		}
	}
	return files, nil
}

func recursiveFiles(ctx context.Context, scope fileSelection, files []string, glob fileGlob) ([]workspaceapp.FileEntry, error) {
	out := make([]workspaceapp.FileEntry, 0, len(files))
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(glob) > 0 {
			candidate, ok := listingRelativePath(f, scope.logical)
			if !ok {
				continue
			}
			if !glob.matches(candidate) {
				continue
			}
		}
		entry, exists, err := scope.inspect(f)
		if err != nil {
			return nil, err
		}
		if exists {
			out = append(out, entry)
		}
	}
	sortFileEntries(out)
	return out, nil
}

func listingRelativePath(file, sub string) (string, bool) {
	if sub == "" {
		return file, true
	}
	if file == sub {
		return path.Base(file), true
	}
	return strings.CutPrefix(file, sub+"/")
}

func levelEntries(scope fileSelection, files []string) ([]workspaceapp.FileEntry, error) {
	prefix := ""
	if scope.logical != "" {
		prefix = scope.logical + "/"
	}
	seenDir := map[string]bool{}
	var children []string
	for _, f := range files {
		rel := f
		if prefix != "" {
			tail, ok := strings.CutPrefix(f, prefix)
			if !ok {
				continue
			}
			rel = tail
		}
		if name, _, nested := strings.Cut(rel, "/"); nested {
			if !seenDir[name] {
				seenDir[name] = true
				children = append(children, path.Join(scope.logical, name))
			}
			continue
		}
		children = append(children, f)
	}
	entries := make([]workspaceapp.FileEntry, 0, len(children))
	for _, child := range children {
		entry, exists, err := scope.inspect(child)
		if err != nil {
			return nil, err
		}
		if exists {
			entries = append(entries, entry)
		}
	}
	sortFileEntries(entries)
	return entries, nil
}

func inspectEntry(root *os.Root, rel string) (workspaceapp.FileEntry, bool, error) {
	info, err := root.Lstat(filepath.FromSlash(rel))
	if errors.Is(err, os.ErrNotExist) {
		// git ls-files includes tracked deletions. They are not current workspace
		// entries, so omit them from the filesystem view.
		return workspaceapp.FileEntry{}, false, nil
	}
	if err != nil {
		return workspaceapp.FileEntry{}, false, fmt.Errorf("inspect %q: %w", rel, err)
	}

	var kind workspaceapp.FileEntryKind
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = workspaceapp.FileEntrySymlink
	case info.IsDir():
		kind = workspaceapp.FileEntryDir
	case info.Mode().IsRegular():
		kind = workspaceapp.FileEntryFile
	default:
		return workspaceapp.FileEntry{}, false, nil
	}
	return workspaceapp.FileEntry{
		Path:       rel,
		Name:       path.Base(rel),
		Kind:       kind,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	}, true, nil
}

func rootPath(relative string) string {
	if relative == "" {
		return "."
	}
	return relative
}

func sortFileEntries(entries []workspaceapp.FileEntry) {
	slices.SortFunc(entries, func(a, b workspaceapp.FileEntry) int {
		return strings.Compare(a.OrderKey(), b.OrderKey())
	})
}

const (
	maxGlobBytes    = 4096
	maxGlobSegments = 128
)

type fileGlob []string

const anyDirectories = "**"

func compileGlob(pattern string) (fileGlob, error) {
	if pattern == "" {
		return nil, nil
	}
	if len(pattern) > maxGlobBytes || path.IsAbs(pattern) || strings.ContainsRune(pattern, 0) || strings.Count(pattern, "/") >= maxGlobSegments {
		return nil, path.ErrBadPattern
	}
	var glob fileGlob
	for part := range strings.SplitSeq(pattern, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, path.ErrBadPattern
		}
		if part == anyDirectories && len(glob) > 0 && glob[len(glob)-1] == anyDirectories {
			continue
		}
		if _, err := path.Match(part, ""); err != nil {
			return nil, err
		}
		glob = append(glob, part)
	}
	return glob, nil
}

func (g fileGlob) matches(candidate string) bool {
	states := make([]bool, len(g)+1)
	states[0] = true
	for i, part := range g {
		states[i+1] = states[i] && part == anyDirectories
	}
	for component := range strings.SplitSeq(candidate, "/") {
		prev := states[0]
		states[0] = false
		for i, part := range g {
			before := states[i+1]
			if part == anyDirectories {
				states[i+1] = before || states[i]
			} else {
				match, _ := path.Match(part, component)
				states[i+1] = prev && match
			}
			prev = before
		}
	}
	return states[len(g)]
}
