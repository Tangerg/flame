package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/samber/lo"
)

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

type FileEntryKind string

const (
	FileEntryFile    FileEntryKind = "file"
	FileEntryDir     FileEntryKind = "dir"
	FileEntrySymlink FileEntryKind = "symlink"
)

type FileEntry struct {
	Path       string
	Name       string
	Kind       FileEntryKind
	SizeBytes  int64
	ModifiedAt time.Time
	orderKey   string
}

func (f FileEntry) OrderKey() string {
	if f.orderKey != "" {
		return f.orderKey
	}
	class := "1"
	if f.Kind == FileEntryDir {
		class = "0"
	}
	return class + ":" + f.Path
}

type FileListOptions struct {
	Path           string
	Glob           string
	NameQuery      *string
	Recursive      bool
	IncludeIgnored bool
}

type fileBrowser interface {
	List(ctx context.Context, root string, options FileListOptions) ([]FileEntry, error)
	ListDirectory(ctx context.Context, root string, plan DirectoryListPlan) (DirectoryPage, error)
	Read(ctx context.Context, root string, plan FileReadPlan) (FileReadResult, error)
	Grep(ctx context.Context, root string, input GrepPlan) (GrepResult, error)
}

type FileListInput struct {
	CWD string
	FileListOptions
	Cursor string
	Limit  pagination.RequestedLimit
}

type FileListCoverage struct {
	ScannedEntries  int
	ExcludedEntries int
	BudgetExhausted bool
}

type DirectoryListPlan struct {
	Path           string
	IncludeIgnored bool
	Offset         int
	Revision       string
	Limit          int
}

type DirectoryPage struct {
	Entries    []FileEntry
	Revision   string
	NextOffset int
	Complete   bool
	FileListCoverage
}

type FilePage struct {
	Complete bool
	FileListCoverage
	Entries    []FileEntry
	NextCursor string
}

type FileReadInput struct {
	Path      string
	Range     FileLineRange
	ByteLimit FileReadByteLimit
	FromEnd   bool
}

type FileReadPlan struct {
	Path      string
	MaxBytes  int
	StartLine int
	EndLine   int
	FromEnd   bool
}

type FileReadResult struct {
	Content    string
	TotalLines int
	StartLine  int
	EndLine    int
	Truncated  bool
}

type GrepInput struct {
	Path    string
	Query   string
	Limit   GrepResultLimit
	ScanAll bool
}

type GrepPlan struct {
	Path    string
	Pattern *regexp.Regexp
	Limit   int
	ScanAll bool
}

type GrepMatch struct {
	Path       string
	LineNumber int
	Text       string
}

type GrepResult struct {
	Matches  []GrepMatch
	Total    int
	Coverage GrepCoverage
}

type GrepCoverage struct {
	ScannedFiles   int
	SkippedFiles   int
	RemainingFiles int
	SourceBytes    int64
}

func (c GrepCoverage) Complete() bool { return c.SkippedFiles == 0 && c.RemainingFiles == 0 }

const (
	defaultFileListPageLimit = 1000
	fileListPageNamespace    = "workspace.files"
	directoryPageNamespace   = "workspace.directory"
)

const (
	DefaultFileReadBytes   = 1 << 20
	maxFileReadBytes       = 8 << 20
	MaxFileReadSourceBytes = 64 << 20
	MaxFileReadLineBytes   = maxFileReadBytes
)

const (
	DefaultGrepLimit         = 100
	MaxGrepLimit             = 1000
	MaxGrepQueryBytes        = 64 << 10
	MaxGrepResultBytes       = 8 << 20
	MaxGrepFileBytes   int64 = 8 << 20
	MaxGrepLineBytes         = 1 << 20
	MaxGrepSourceBytes       = 512 << 20
)

func (f *Files) listDirectory(ctx context.Context, root, path string, input FileListInput) (FilePage, error) {
	size, err := input.Limit.Resolve(defaultFileListPageLimit)
	if err != nil {
		return FilePage{}, fmt.Errorf("%w: %w", ErrPageLimit, err)
	}
	filters := []string{root, path, strconv.FormatBool(input.IncludeIgnored)}
	anchor, err := pagination.Decode(input.Cursor, directoryPageNamespace, filters)
	if err != nil {
		return FilePage{}, fmt.Errorf("%w: %w", ErrPageCursor, err)
	}
	plan := DirectoryListPlan{Path: path, IncludeIgnored: input.IncludeIgnored, Limit: size}
	if len(anchor) > 0 {
		if len(anchor) != 2 {
			return FilePage{}, ErrPageCursor
		}
		plan.Offset, err = strconv.Atoi(anchor[0])
		if err != nil || plan.Offset < 0 {
			return FilePage{}, ErrPageCursor
		}
		plan.Revision = anchor[1]
	}
	page, err := f.files.ListDirectory(ctx, root, plan)
	if err != nil {
		return FilePage{}, err
	}
	next := ""
	if !page.Complete {
		next, err = pagination.Encode(directoryPageNamespace, filters, []string{strconv.Itoa(page.NextOffset), page.Revision})
		if err != nil {
			return FilePage{}, fmt.Errorf("%w: %w", ErrPageCursor, err)
		}
	}
	return FilePage{Entries: page.Entries, NextCursor: next, Complete: page.Complete, FileListCoverage: page.FileListCoverage}, nil
}

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
	if input.NameQuery != nil && len(*input.NameQuery) > maxNameQueryBytes {
		return FilePage{}, ErrInvalidFileNameQuery
	}
	if !input.Recursive && input.Glob == "" && input.NameQuery == nil {
		return f.listDirectory(ctx, root, path, input)
	}
	entries, err := f.files.List(ctx, root, FileListOptions{
		Path: path, Glob: input.Glob, Recursive: input.Recursive || input.NameQuery != nil, IncludeIgnored: input.IncludeIgnored,
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
	if input.NameQuery != nil {
		filters = append(filters, *input.NameQuery)
		entries, err = rankedFileNames(ctx, entries, *input.NameQuery)
		if err != nil {
			return FilePage{}, err
		}
	}
	page, next, err := pageFileEntries(entries, filters, input.Cursor, input.Limit)
	if err != nil {
		return FilePage{}, err
	}
	return FilePage{Entries: page, NextCursor: next, Complete: next == "", FileListCoverage: FileListCoverage{ScannedEntries: len(entries)}}, nil
}

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
	return f.files.Read(ctx, root, FileReadPlan{
		Path: path, StartLine: start, EndLine: end, MaxBytes: input.ByteLimit.Bytes(), FromEnd: input.FromEnd,
	})
}

func (f *Files) Grep(ctx context.Context, cwd string, input GrepInput) (GrepResult, error) {
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
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
	return f.files.Grep(ctx, root, GrepPlan{Path: input.Path, Pattern: pattern, Limit: input.Limit.Matches(), ScanAll: input.ScanAll})
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

const maxNameQueryBytes = 4096

func rankedFileNames(ctx context.Context, entries []FileEntry, raw string) ([]FileEntry, error) {
	query := []rune(strings.ToLower(raw))
	ranked := entries[:0]
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Kind != FileEntryFile {
			continue
		}
		if len(query) > 0 {
			name := strings.ToLower(entry.Path)
			score := subsequenceScore(query, path.Base(name))
			group := 1
			if score == 0 {
				group = 2
				score = subsequenceScore(query, name)
			}
			if score == 0 {
				continue
			}
			if entry.Path == raw {
				group = 0
			}
			const maximumRuneScore = 6
			entry.orderKey = fmt.Sprintf("%d:%08x:%08x:%s", group, maximumRuneScore*len(query)-score, utf8.RuneCountInString(entry.Path), entry.Path)
		}
		ranked = append(ranked, entry)
	}
	return ranked, nil
}

func subsequenceScore(query []rune, name string) int {
	var matched, score int
	boundary, adjacent := true, false
	for _, ch := range name {
		if ch == query[matched] {
			score++
			if adjacent {
				score += 3
			}
			if boundary {
				score += 2
			}
			matched++
			if matched == len(query) {
				return score
			}
			adjacent = true
		} else {
			adjacent = false
		}
		boundary = strings.ContainsRune("/._-", ch)
	}
	return 0
}
