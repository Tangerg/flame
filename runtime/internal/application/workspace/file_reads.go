package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/samber/lo"
)

// Files owns root-scoped file browser operations.
type Files struct {
	scope *Scope
	files fileBrowser
}

func NewFiles(scope *Scope, files fileBrowser) (*Files, error) {
	if scope == nil {
		return nil, errors.New("workspace: file scope is required")
	}
	if lo.IsNil(files) {
		return nil, errors.New("workspace: file browser is required")
	}
	return &Files{scope: scope, files: files}, nil
}

// FileEntryKind identifies a file-system entry in the workspace browser.
type FileEntryKind string

const (
	FileEntryFile    FileEntryKind = "file"
	FileEntryDir     FileEntryKind = "dir"
	FileEntrySymlink FileEntryKind = "symlink"
)

// FileEntry is an inspected, root-relative workspace entry.
type FileEntry struct {
	Path       string
	Name       string
	Kind       FileEntryKind
	SizeBytes  int64
	ModifiedAt time.Time
}

// OrderKey is the stable ordering and pagination identity for a listing.
func (f FileEntry) OrderKey() string {
	class := "1"
	if f.Kind == FileEntryDir {
		class = "0"
	}
	return class + ":" + f.Path
}

// FileListOptions controls one workspace file listing.
type FileListOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

// FileBrowser is the application-owned port for workspace file reads.
type fileBrowser interface {
	List(ctx context.Context, root string, options FileListOptions) ([]FileEntry, error)
	Read(ctx context.Context, root string, plan FileReadPlan) (FileReadResult, error)
	Grep(ctx context.Context, root string, input GrepPlan) (GrepResult, error)
}

// FileListInput is one paged workspace listing request.
type FileListInput struct {
	CWD string
	FileListOptions
	Cursor string
	Limit  pagination.RequestedLimit
}

// FilePage is a stable cursor page of workspace entries.
type FilePage struct {
	Entries    []FileEntry
	NextCursor string
}

// FileReadInput specifies a root-relative complete-line read. FromEnd retains
// the suffix of Range within ByteLimit, without reversing source order.
type FileReadInput struct {
	Path      string
	Range     FileLineRange
	ByteLimit FileReadByteLimit
	FromEnd   bool
}

// FileReadPlan is the fully normalized primitive filesystem-port request. Read
// policy has already resolved default/clamp/presence before this value exists.
type FileReadPlan struct {
	Path      string
	MaxBytes  int
	StartLine int
	EndLine   int
	FromEnd   bool
}

// FileReadResult is a file read with whole-file line information.
type FileReadResult struct {
	Content    string
	TotalLines int
	StartLine  int // zero-based
	EndLine    int // one-based inclusive
	Truncated  bool
}

// GrepInput specifies one root-scoped content search.
type GrepInput struct {
	Path  string
	Query string
	Limit GrepResultLimit
}

// GrepPlan is the normalized, compiled search accepted by the filesystem port.
// Keeping it separate from GrepInput prevents the filesystem port from
// receiving raw, unbounded regular expressions or caller-owned result limits.
type GrepPlan struct {
	Path    string
	Pattern *regexp.Regexp
	Limit   int
}

// GrepMatch is one content-search match.
type GrepMatch struct {
	Path       string
	LineNumber int
	Text       string
}

// GrepResult is a capped content search plus its honest total match count.
type GrepResult struct {
	Matches []GrepMatch
	Total   int
}

const (
	defaultFileListPageLimit = 1000
	fileListPageNamespace    = "workspace.files"
)

// Workspace file reads retain at most 1 MiB by default and 8 MiB when a
// caller explicitly raises the budget. The full-file scan is admitted only up
// to 64 MiB because TotalLines describes the complete source, while no single
// editor-facing line may exceed the largest possible response.
const (
	DefaultFileReadBytes   = 1 << 20
	maxFileReadBytes       = 8 << 20
	MaxFileReadSourceBytes = 64 << 20
	MaxFileReadLineBytes   = maxFileReadBytes
)

// Workspace search returns an exact total over a finite searchable corpus and
// retains only a bounded, whole-row prefix for its caller. Oversized/non-text
// individual files are outside that corpus; crossing its aggregate source or
// candidate boundary fails explicitly instead of publishing a partial total.
const (
	DefaultGrepLimit         = 100
	MaxGrepLimit             = 1000
	MaxGrepQueryBytes        = 64 << 10
	MaxGrepResultBytes       = 8 << 20
	MaxGrepFileBytes   int64 = 8 << 20
	MaxGrepLineBytes         = 1 << 20
	MaxGrepSourceBytes       = 512 << 20
)

// List returns one stable cursor page of entries below a workspace root.
func (f *Files) List(ctx context.Context, input FileListInput) (FilePage, error) {
	root, err := f.scope.ResolveRoot(input.CWD)
	if err != nil {
		return FilePage{}, err
	}
	path := ""
	if input.Path != "" {
		// Listing a subdirectory dereferences it. Resolve its physical identity at
		// the application boundary just like read/search, otherwise a lexical
		// in-root symlink could make the file browser port enumerate a directory
		// outside the admitted workspace.
		path, err = f.scope.paths.ResolveExistingInRoot(root, input.Path)
		if err != nil {
			return FilePage{}, err
		}
	}
	entries, err := f.files.List(ctx, root, FileListOptions{
		Path: path, Glob: input.Glob, Recursive: input.Recursive, IncludeIgnored: input.IncludeIgnored,
	})
	if err != nil {
		return FilePage{}, err
	}
	filters := []string{
		root,
		path,
		input.Glob,
		strconv.FormatBool(input.Recursive),
		strconv.FormatBool(input.IncludeIgnored),
	}
	page, next, err := pageFileEntries(entries, filters, input.Cursor, input.Limit)
	if err != nil {
		return FilePage{}, err
	}
	return FilePage{Entries: page, NextCursor: next}, nil
}

// Read returns all or a one-based inclusive line window of a workspace
// file. Its range and byte budget were settled when they were constructed.
func (f *Files) Read(ctx context.Context, cwd string, input FileReadInput) (FileReadResult, error) {
	if input.Path == "" {
		return FileReadResult{}, ErrPathRequired
	}
	root, err := f.scope.ResolveRoot(cwd)
	if err != nil {
		return FileReadResult{}, err
	}
	path, err := f.scope.paths.ResolveExistingInRoot(root, input.Path)
	if err != nil {
		return FileReadResult{}, err
	}
	start, end := input.Range.Bounds()
	return f.readFile(ctx, root, FileReadPlan{
		Path: path, StartLine: start, EndLine: end, MaxBytes: input.ByteLimit.Bytes(), FromEnd: input.FromEnd,
	})
}

func (f *Files) readFile(ctx context.Context, root string, input FileReadPlan) (FileReadResult, error) {
	return f.files.Read(ctx, root, input)
}

// Grep searches a workspace root or an existing subdirectory. A truncated
// search returns an honest total rather than silently under-reporting hits.
func (f *Files) Grep(ctx context.Context, cwd string, input GrepInput) (GrepResult, error) {
	if cause := cancellation.Err(ctx); cause != nil {
		return GrepResult{}, cause
	}
	if input.Query == "" {
		return GrepResult{}, ErrGrepQueryMissing
	}
	if len(input.Query) > MaxGrepQueryBytes || !utf8.ValidString(input.Query) || strings.ContainsRune(input.Query, 0) {
		return GrepResult{}, ErrInvalidGrepQuery
	}
	pattern, err := regexp.Compile(input.Query)
	if err != nil {
		return GrepResult{}, fmt.Errorf("%w: %w", ErrInvalidGrepQuery, err)
	}
	root, err := f.scope.ResolveRoot(cwd)
	if err != nil {
		return GrepResult{}, err
	}
	if input.Path != "" {
		input.Path, err = f.scope.paths.ResolveExistingInRoot(root, input.Path)
		if err != nil {
			return GrepResult{}, err
		}
	}
	return f.files.Grep(ctx, root, GrepPlan{Path: input.Path, Pattern: pattern, Limit: input.Limit.Matches()})
}

func pageFileEntries(entries []FileEntry, filters []string, cursor string, limit pagination.RequestedLimit) ([]FileEntry, string, error) {
	size, err := limit.Resolve(defaultFileListPageLimit)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrPageLimit, err)
	}
	slices.SortFunc(entries, func(a, b FileEntry) int { return cmp.Compare(a.OrderKey(), b.OrderKey()) })
	anchor, err := pagination.Decode(cursor, fileListPageNamespace, filters)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrPageCursor, err)
	}
	if len(anchor) > 0 {
		if len(anchor) != 1 {
			return nil, "", ErrPageCursor
		}
		start := len(entries)
		for index, entry := range entries {
			if candidate := entry.OrderKey(); candidate >= anchor[0] {
				start = index
				if candidate == anchor[0] {
					start++
				}
				break
			}
		}
		entries = entries[start:]
	}
	end := min(size, len(entries))
	page := slices.Clone(entries[:end])
	if end == len(entries) {
		return page, "", nil
	}
	nextCursor, err := pagination.Encode(fileListPageNamespace, filters, []string{entries[end-1].OrderKey()})
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrPageCursor, err)
	}
	return page, nextCursor, nil
}
