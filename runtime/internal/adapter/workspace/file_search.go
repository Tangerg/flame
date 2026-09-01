package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/tools/textread"
)

// Grep searches the same finite, ignore-aware file catalog exposed by the
// workspace browser. It scans in process so source, line, retained material,
// cancellation, and exact-total semantics remain owned by this product port
// instead of inheriting a subprocess executor's post-hoc result slicing.
func (FileBrowser) Grep(ctx context.Context, root string, input workspaceapp.GrepPlan) (workspaceapp.GrepResult, error) {
	if cause := context.Cause(ctx); cause != nil {
		return workspaceapp.GrepResult{}, cause
	}
	if input.Pattern == nil || input.Limit <= 0 || input.Limit > workspaceapp.MaxGrepLimit {
		return workspaceapp.GrepResult{}, workspaceapp.ErrInvalidGrepQuery
	}
	entries, err := ListFiles(ctx, root, workspaceapp.FileListOptions{Path: input.Path, Recursive: true})
	if err != nil {
		if errors.Is(err, ErrListingTooLarge) {
			return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
		}
		return workspaceapp.GrepResult{}, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.GrepResult{}, fmt.Errorf("workspace: resolve search root: %w", err)
	}
	search := workspaceGrep{
		ctx:             ctx,
		root:            root,
		canonicalRoot:   canonicalRoot,
		plan:            input,
		result:          workspaceapp.GrepResult{Matches: []workspaceapp.GrepMatch{}},
		remainingSource: int64(workspaceapp.MaxGrepSourceBytes),
		collectMatches:  true,
	}
	for _, entry := range entries {
		if err := search.scanEntry(entry); err != nil {
			return workspaceapp.GrepResult{}, err
		}
	}
	return search.result, nil
}

type workspaceGrep struct {
	ctx             context.Context
	root            string
	canonicalRoot   string
	plan            workspaceapp.GrepPlan
	result          workspaceapp.GrepResult
	remainingSource int64
	retainedBytes   int
	collectMatches  bool
}

func (search *workspaceGrep) scanEntry(entry workspaceapp.FileEntry) error {
	if entry.Kind != workspaceapp.FileEntryFile || entry.SizeBytes > workspaceapp.MaxGrepFileBytes {
		return nil
	}
	if cause := context.Cause(search.ctx); cause != nil {
		return cause
	}
	source, usable, err := search.openSource(entry.Path)
	if err != nil || !usable {
		return err
	}
	return search.scanSource(source)
}

type grepSource struct {
	path string
	file *os.File
}

func (search *workspaceGrep) openSource(entryPath string) (grepSource, bool, error) {
	path, err := rootRelativeFilePath(
		search.root,
		search.canonicalRoot,
		filepath.Join(search.root, filepath.FromSlash(entryPath)),
	)
	if err != nil {
		return grepSource{}, false, err
	}
	file, err := openRegularFile(
		filepath.Join(search.canonicalRoot, filepath.FromSlash(path)),
		workspaceapp.MaxGrepFileBytes,
	)
	if err != nil {
		if errors.Is(err, errFileSourceNotRegular) || errors.Is(err, errFileSourceTooLarge) {
			return grepSource{}, false, nil
		}
		return grepSource{}, false, fmt.Errorf("workspace: open search file %q: %w", path, err)
	}
	return grepSource{path: path, file: file}, true, nil
}

func (search *workspaceGrep) scanSource(source grepSource) error {
	if search.remainingSource <= 0 {
		_ = source.file.Close()
		return workspaceapp.ErrGrepResultTooLarge
	}
	counter := &searchByteCounter{reader: source.file}
	matchLimit, resultBytes := search.retentionBudget()
	fileResult, scanErr := grepFile(
		search.ctx,
		counter,
		source.path,
		search.plan,
		min(workspaceapp.MaxGrepFileBytes, search.remainingSource),
		matchLimit,
		resultBytes,
	)
	if err := source.file.Close(); err != nil {
		return fmt.Errorf("workspace: close search file %q: %w", source.path, err)
	}
	search.remainingSource -= counter.bytes
	if search.remainingSource < 0 {
		return workspaceapp.ErrGrepResultTooLarge
	}
	if err := classifyGrepScanError(source.path, scanErr); err != nil {
		return err
	}
	if scanErr != nil {
		// Binary and pathological single-line files are not members of the
		// searchable text corpus. Discard the whole file, including any rows
		// observed before invalid material, just as a binary-aware grep does.
		return nil
	}
	return search.merge(fileResult)
}

func (search *workspaceGrep) retentionBudget() (int, int) {
	if !search.collectMatches {
		return 0, 0
	}
	return search.plan.Limit - len(search.result.Matches),
		workspaceapp.MaxGrepResultBytes - search.retainedBytes
}

func (search *workspaceGrep) merge(fileResult grepFileResult) error {
	if fileResult.total > math.MaxInt-search.result.Total {
		return workspaceapp.ErrGrepResultTooLarge
	}
	search.result.Total += fileResult.total
	search.result.Matches = append(search.result.Matches, fileResult.matches...)
	search.retainedBytes += fileResult.materialBytes
	if fileResult.exhausted {
		search.collectMatches = false
	}
	return nil
}

func classifyGrepScanError(path string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, textread.ErrInvalidText), errors.Is(err, textread.ErrLineTooLarge):
		return nil
	case errors.Is(err, textread.ErrInputTooLarge):
		return workspaceapp.ErrGrepResultTooLarge
	default:
		return fmt.Errorf("workspace: scan search file %q: %w", path, err)
	}
}

type grepFileResult struct {
	matches       []workspaceapp.GrepMatch
	total         int
	materialBytes int
	exhausted     bool
}

func grepFile(
	ctx context.Context,
	reader io.Reader,
	path string,
	input workspaceapp.GrepPlan,
	inputBytes int64,
	matchLimit int,
	resultBytes int,
) (grepFileResult, error) {
	result := grepFileResult{matches: []workspaceapp.GrepMatch{}}
	err := textread.VisitLines(ctx, reader, textread.Limits{
		InputBytes: inputBytes,
		LineBytes:  workspaceapp.MaxGrepLineBytes,
	}, func(number int, line []byte) error {
		if !input.Pattern.Match(line) {
			return nil
		}
		result.total++
		if result.exhausted {
			return nil
		}
		rowBytes := len(path) + len(line)
		if len(result.matches) >= matchLimit || rowBytes > resultBytes-result.materialBytes {
			result.exhausted = true
			return nil
		}
		result.matches = append(result.matches, workspaceapp.GrepMatch{
			Path: path, LineNumber: number, Text: string(line),
		})
		result.materialBytes += rowBytes
		return nil
	})
	return result, err
}

// rootRelativeFilePath converts a host path into a confined slash-separated
// workspace identity. Canonicalizing both sides also rejects a candidate that
// escapes through an in-root symlink.
func rootRelativeFilePath(root, canonicalRoot, candidate string) (string, error) {
	abs := candidate
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, candidate)
	}
	canonicalCandidate, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve search candidate %q: %w", candidate, err)
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("workspace: relativize search candidate %q: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return filepath.ToSlash(rel), nil
}

type searchByteCounter struct {
	reader io.Reader
	bytes  int64
}

func (s *searchByteCounter) Read(buffer []byte) (int, error) {
	read, err := s.reader.Read(buffer)
	s.bytes += int64(read)
	return read, err
}
