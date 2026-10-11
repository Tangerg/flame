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
	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/scope/tools/textread"
)

// Grep searches the same finite, ignore-aware file catalog exposed by the
// workspace browser. It scans in process so source, line, retained material,
// cancellation, and exact-total semantics remain owned by this product port
// instead of inheriting a subprocess executor's post-hoc result slicing.
func (FileBrowser) Grep(ctx context.Context, root string, input workspaceapp.GrepPlan) (_ workspaceapp.GrepResult, err error) {
	if cause := cancellation.Err(ctx); cause != nil {
		return workspaceapp.GrepResult{}, cause
	}
	if input.Pattern == nil || input.Limit <= 0 || input.Limit > workspaceapp.MaxGrepLimit {
		return workspaceapp.GrepResult{}, workspaceapp.ErrInvalidGrepQuery
	}
	entries, err := SearchFiles(ctx, root, input.Path, "")
	if err != nil {
		if errors.Is(err, ErrListingTooLarge) {
			return workspaceapp.GrepResult{}, errors.Join(workspaceapp.ErrGrepResultTooLarge, err)
		}
		return workspaceapp.GrepResult{}, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.GrepResult{}, fmt.Errorf("workspace: resolve search root: %w", err)
	}
	rootHandle, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return workspaceapp.GrepResult{}, fmt.Errorf("workspace: open search root: %w", err)
	}
	defer func() {
		err = errors.Join(err, rootHandle.Close())
	}()
	search := workspaceGrep{
		ctx:             ctx,
		root:            root,
		canonicalRoot:   canonicalRoot,
		rootHandle:      rootHandle,
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
	rootHandle      *os.Root
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
	if cause := cancellation.Err(search.ctx); cause != nil {
		return cause
	}
	source, usable, err := search.openSource(entry.Path)
	if err != nil || !usable {
		return err
	}
	return search.scanSource(source)
}

type grepSource struct {
	path   string
	file   *os.File
	opened os.FileInfo
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
	file, opened, err := fileinput.OpenAt(
		search.rootHandle,
		filepath.FromSlash(path),
		workspaceapp.MaxGrepFileBytes,
	)
	if err != nil {
		if errors.Is(err, fileinput.ErrNotRegular) || errors.Is(err, fileinput.ErrTooLarge) {
			return grepSource{}, false, nil
		}
		return grepSource{}, false, fmt.Errorf("workspace: open search file %q: %w", path, err)
	}
	return grepSource{path: path, file: file, opened: opened}, true, nil
}

func (search *workspaceGrep) scanSource(source grepSource) error {
	if search.remainingSource <= 0 {
		return errors.Join(workspaceapp.ErrGrepResultTooLarge, source.file.Close(), cancellation.Err(search.ctx))
	}
	matchLimit, resultBytes := search.retentionBudget()
	fileResult, scanErr := grepFile(
		search.ctx,
		source.file,
		source.path,
		search.plan,
		min(workspaceapp.MaxGrepFileBytes, search.remainingSource),
		matchLimit,
		resultBytes,
	)
	verifyErr := fileinput.VerifyAtVersion(
		source.file,
		source.opened,
		search.rootHandle,
		filepath.FromSlash(source.path),
	)
	if verifyErr != nil {
		verifyErr = fmt.Errorf("workspace: verify search file %q after reading: %w", source.path, verifyErr)
	}
	closeErr := source.file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("workspace: close search file %q: %w", source.path, closeErr)
	}
	search.remainingSource -= fileResult.sourceBytes
	var capacityErr error
	if search.remainingSource < 0 {
		capacityErr = workspaceapp.ErrGrepResultTooLarge
	}
	if err := errors.Join(scanErr, capacityErr, verifyErr, closeErr, cancellation.Err(search.ctx)); err != nil {
		return err
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

type grepFileResult struct {
	matches       []workspaceapp.GrepMatch
	total         int
	materialBytes int
	exhausted     bool
	sourceBytes   int64
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
	counter := &fileScanInput{reader: reader}
	err := textread.VisitLines(ctx, counter, textread.Limits{
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
	result.sourceBytes = counter.bytes
	if errors.Is(err, textread.ErrInputTooLarge) {
		err = errors.Join(workspaceapp.ErrGrepResultTooLarge, err)
	}
	if inputErr := errors.Join(counter.readErr, cancellation.Err(ctx)); inputErr != nil {
		return result, fmt.Errorf("workspace: scan search file %q: %w", path, errors.Join(err, inputErr))
	}
	switch {
	case errors.Is(err, textread.ErrInvalidText), errors.Is(err, textread.ErrLineTooLarge):
		// Exclude the entire file only after ruling out input failure: the
		// parser may diagnose buffered text before reaching a reader error.
		return grepFileResult{sourceBytes: counter.bytes}, nil
	case err != nil:
		return result, fmt.Errorf("workspace: scan search file %q: %w", path, err)
	}
	return result, nil
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

// Buffered text admission can stop before delivering a pending reader error.
// These observations belong to actual I/O, regardless of the parser's result.
type fileScanInput struct {
	reader  io.Reader
	bytes   int64
	readErr error
}

func (s *fileScanInput) Read(buffer []byte) (int, error) {
	read, err := s.reader.Read(buffer)
	s.bytes += int64(read)
	if err != nil && err != io.EOF {
		s.readErr = errors.Join(s.readErr, err)
	}
	return read, err
}
