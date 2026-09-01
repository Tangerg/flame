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

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

// File browsing returns workspace entries for tree and completion consumers.
// Listing is gitignore-aware: in a git repo the
// candidate set comes from `git ls-files` (tracked + untracked-not-ignored, the
// repo's own .gitignore as authority); outside a repo it's a filesystem walk
// that skips a backstop set of heavy build/vcs dirs. This package owns Git
// interaction so callers depend only on listing behavior.

var (
	// ErrListingTooLarge asks the caller to narrow Path or Glob instead of
	// returning an incomplete result that looks authoritative.
	ErrListingTooLarge = errors.New("workspace: file listing too large")
	// ErrInvalidGlob distinguishes malformed match syntax from a valid pattern
	// that simply has no matching files.
	ErrInvalidGlob = errors.New("workspace: invalid file glob")
	// errInvalidListPath reports a missing or non-directory selected listing
	// root. Permission and I/O failures remain operational errors.
	errInvalidListPath = errors.New("workspace: invalid file listing path")
)

// maxListEntries is a safety boundary, not a silent result cap. Crossing it
// returns ErrListingTooLarge so callers can report precise invalid input.
const maxListEntries = 20000

// backstopExclude are directories never worth listing. `.git` is always
// skipped (even with includeIgnored — its internals are never useful); the
// rest are skipped only when not includeIgnored, as a coarse stand-in for
// .gitignore outside a git repo.
var backstopExclude = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "dist": true,
	"build": true, "target": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, ".idea": true, ".vscode": true,
	".cache": true, "coverage": true, ".turbo": true, ".svn": true, ".hg": true,
}

// ListFiles lists entries under opts.Path within root. With Recursive (or a
// Glob) it returns a flat list of files for the subtree; otherwise the
// immediate children (files + dirs) of opts.Path, for a lazy file tree.
// The complete, deterministically ordered result is returned for use-case
// pagination. Oversized trees fail explicitly with ErrListingTooLarge.
func ListFiles(ctx context.Context, root string, opts workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	sub := path.Clean(filepath.ToSlash(opts.Path))
	if sub == "." || sub == "/" {
		sub = ""
	}
	scope, err := resolveListDirectory(root, sub)
	if err != nil {
		return nil, err
	}
	if opts.Glob != "" {
		if _, err := matchGlob(opts.Glob, ""); err != nil {
			return nil, fmt.Errorf("%w %q: %v", ErrInvalidGlob, opts.Glob, err)
		}
	}
	repository := false
	var files []string
	if !opts.IncludeIgnored {
		files, err = git.ListFiles(ctx, scope.root, scope.physical, maxListEntries)
		switch {
		case err == nil:
			repository = true
			files = scope.project(files)
		case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
			// Git-aware listing is unavailable for this workspace. The
			// filesystem fallback below remains authoritative in this case.
		case errors.Is(err, git.ErrResultTooLarge):
			return nil, fmt.Errorf("%w: more than %d files", ErrListingTooLarge, maxListEntries)
		default:
			return nil, err
		}
	}
	// A non-recursive filesystem listing is genuinely one level. Walking the
	// entire subtree first defeats lazy tree loading: a home-directory workspace
	// can hit an unreadable descendant or the global safety limit before its
	// immediate children are returned. Git-backed listings still derive their
	// children from `git ls-files` so ignored directories stay hidden.
	if !opts.Recursive && opts.Glob == "" && !repository {
		return levelFilesystemEntries(ctx, scope, opts.IncludeIgnored)
	}

	if !repository {
		files, err = walkFiles(ctx, scope, opts.IncludeIgnored)
		if err != nil {
			return nil, err
		}
	}
	if len(files) > maxListEntries {
		return nil, fmt.Errorf("%w: more than %d files", ErrListingTooLarge, maxListEntries)
	}

	if opts.Recursive || opts.Glob != "" {
		return recursiveFiles(scope, files, opts.Glob)
	}
	return levelEntries(scope, files)
}

type listDirectory struct {
	root     string
	logical  string
	physical string
	path     string
}

func resolveListDirectory(root, sub string) (listDirectory, error) {
	if path.IsAbs(sub) || sub == ".." || strings.HasPrefix(sub, "../") {
		return listDirectory{}, fmt.Errorf("%w: %q escapes the workspace", errInvalidListPath, sub)
	}
	physicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return listDirectory{}, fmt.Errorf("inspect listing root %q: %w", root, err)
	}
	physicalPath, err := pathidentity.Resolve(physicalRoot, filepath.FromSlash(sub))
	if err != nil {
		return listDirectory{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	inside, err := pathidentity.Contains(physicalRoot, physicalPath)
	if err != nil {
		return listDirectory{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if !inside {
		return listDirectory{}, fmt.Errorf("%w: %q escapes the workspace", errInvalidListPath, sub)
	}
	info, err := os.Stat(physicalPath)
	if errors.Is(err, os.ErrNotExist) {
		return listDirectory{}, fmt.Errorf("%w: %q does not exist", errInvalidListPath, sub)
	}
	if err != nil {
		return listDirectory{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if !info.IsDir() {
		return listDirectory{}, fmt.Errorf("%w: %q is not a directory", errInvalidListPath, sub)
	}
	physical, err := filepath.Rel(physicalRoot, physicalPath)
	if err != nil {
		return listDirectory{}, fmt.Errorf("inspect listing path %q: %w", sub, err)
	}
	if physical == "." {
		physical = ""
	}
	return listDirectory{
		root: physicalRoot, logical: sub, physical: filepath.ToSlash(physical), path: physicalPath,
	}, nil
}

func (l listDirectory) project(files []string) []string {
	if l.logical == l.physical {
		return files
	}
	projected := make([]string, 0, len(files))
	for _, file := range files {
		relative, ok := listingRelativePath(file, l.physical)
		if !ok {
			continue
		}
		projected = append(projected, path.Join(l.logical, relative))
	}
	return projected
}

func (l listDirectory) inspect(logical string) (workspaceapp.FileEntry, bool, error) {
	physical := logical
	if l.logical != l.physical {
		relative, ok := listingRelativePath(logical, l.logical)
		if !ok {
			return workspaceapp.FileEntry{}, false, nil
		}
		physical = path.Join(l.physical, relative)
	}
	entry, exists, err := inspectEntry(l.root, physical)
	if err != nil || !exists {
		return entry, exists, err
	}
	entry.Path = logical
	entry.Name = path.Base(logical)
	return entry, true, nil
}

func levelFilesystemEntries(ctx context.Context, scope listDirectory, includeIgnored bool) ([]workspaceapp.FileEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	children, err := readDirectoryEntries(scope.path, maxListEntries)
	if err != nil {
		return nil, err
	}
	entries := make([]workspaceapp.FileEntry, 0, len(children))
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if child.Name() == ".git" || (child.IsDir() && !includeIgnored && backstopExclude[child.Name()]) {
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

func readDirectoryEntries(directory string, limit int) ([]fs.DirEntry, error) {
	source, err := os.Stat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q does not exist", errInvalidListPath, directory)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect listing path %q: %w", directory, err)
	}
	if !source.IsDir() {
		return nil, fmt.Errorf("%w: %q is not a directory", errInvalidListPath, directory)
	}
	dir, _, err := fileinput.OpenDirectoryExpected(directory, source)
	if err != nil {
		if errors.Is(err, fileinput.ErrNotDirectory) || errors.Is(err, fileinput.ErrChanged) {
			return nil, fmt.Errorf("%w: %q changed while it was being opened", errInvalidListPath, directory)
		}
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	defer func() { _ = dir.Close() }()

	// Read one sentinel entry beyond the contract limit. os.ReadDir(directory)
	// would materialize an unbounded directory before the safety policy had a
	// chance to reject it.
	children, err := dir.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	if len(children) > limit {
		return nil, fmt.Errorf("%w: more than %d entries in %q", ErrListingTooLarge, limit, directory)
	}
	return children, nil
}

// walkFiles is the non-repo fallback: a filesystem walk under root/sub that
// skips backstop directories and fails explicitly at the safety boundary.
func walkFiles(ctx context.Context, scope listDirectory, includeIgnored bool) ([]string, error) {
	var files []string
	walkErr := filepath.WalkDir(scope.path, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return fmt.Errorf("visit %q: %w", p, err)
		}
		if p != scope.path && d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != scope.path && !includeIgnored && backstopExclude[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, e := filepath.Rel(scope.path, p)
		if e != nil {
			return nil
		}
		files = append(files, path.Join(scope.logical, filepath.ToSlash(rel)))
		if len(files) > maxListEntries {
			return ErrListingTooLarge
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %q: %w", scope.path, walkErr)
	}
	return files, nil
}

// recursiveFiles turns flat candidate paths into inspected file entries.
func recursiveFiles(scope listDirectory, files []string, glob string) ([]workspaceapp.FileEntry, error) {
	out := make([]workspaceapp.FileEntry, 0, len(files))
	for _, f := range files {
		if glob != "" {
			candidate, ok := listingRelativePath(f, scope.logical)
			if !ok {
				continue
			}
			matched, err := matchGlob(glob, candidate)
			if err != nil {
				return nil, fmt.Errorf("%w %q: %v", ErrInvalidGlob, glob, err)
			}
			if !matched {
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

// levelEntries derives the immediate children of sub from the flat candidate
// paths: direct files become file entries, and any deeper path contributes its
// first path segment as a dir entry (deduped). Dirs sort before files.
func levelEntries(scope listDirectory, files []string) ([]workspaceapp.FileEntry, error) {
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

func inspectEntry(root, rel string) (workspaceapp.FileEntry, bool, error) {
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
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

func sortFileEntries(entries []workspaceapp.FileEntry) {
	slices.SortFunc(entries, func(a, b workspaceapp.FileEntry) int {
		return strings.Compare(a.OrderKey(), b.OrderKey())
	})
}

// matchGlob matches a slash-separated workspace path. A complete segment named
// ** consumes zero or more path segments; every other segment follows
// path.Match syntax. Keeping the matcher here gives every finite-catalog
// consumer one glob dialect without adding a second filesystem walker.
func matchGlob(pattern, relPath string) (bool, error) {
	if pattern == "" || path.IsAbs(pattern) || strings.ContainsRune(pattern, 0) {
		return false, path.ErrBadPattern
	}
	patternParts := strings.Split(pattern, "/")
	for _, part := range patternParts {
		if part == "" || part == "." || part == ".." {
			return false, path.ErrBadPattern
		}
		if part != "**" {
			if _, err := path.Match(part, ""); err != nil {
				return false, err
			}
		}
	}
	pathParts := []string{}
	if relPath != "" {
		pathParts = strings.Split(relPath, "/")
	}

	type state struct{ pattern, path int }
	known := make(map[state]bool)
	memo := make(map[state]bool)
	var match func(int, int) (bool, error)
	match = func(patternIndex, pathIndex int) (bool, error) {
		key := state{pattern: patternIndex, path: pathIndex}
		if known[key] {
			return memo[key], nil
		}
		known[key] = true
		if patternIndex == len(patternParts) {
			memo[key] = pathIndex == len(pathParts)
			return memo[key], nil
		}
		if patternParts[patternIndex] == "**" {
			matched, err := match(patternIndex+1, pathIndex)
			if err != nil || matched {
				memo[key] = matched
				return matched, err
			}
			if pathIndex < len(pathParts) {
				matched, err = match(patternIndex, pathIndex+1)
				memo[key] = matched
				return matched, err
			}
			return false, nil
		}
		if pathIndex >= len(pathParts) {
			return false, nil
		}
		matched, err := path.Match(patternParts[patternIndex], pathParts[pathIndex])
		if err != nil || !matched {
			return false, err
		}
		matched, err = match(patternIndex+1, pathIndex+1)
		memo[key] = matched
		return matched, err
	}
	return match(0, 0)
}
