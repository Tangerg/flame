package git

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SummarizePatch parses a git-compatible unified diff into per-file summaries
// (path, status, previous path, and added/removed line counts) without
// retaining rows or applying a materialization budget. It owns the one patch
// grammar every client reads from, so a tool projection can report what a
// model-authored patch proposes without each consumer re-implementing the
// parser. The input is a bounded tool-call argument, so no row budget applies.
func SummarizePatch(patch []byte) ([]DiffFile, error) {
	parser := unifiedDiffParser{maxFiles: math.MaxInt, maxRows: math.MaxInt}
	for encoded := range bytes.SplitSeq(patch, []byte{'\n'}) {
		if _, err := parser.consume(string(encoded)); err != nil {
			return nil, err
		}
	}
	if _, err := parser.flush(); err != nil {
		return nil, err
	}
	for i := range parser.files {
		parser.files[i].Rows = nil
	}
	return parser.files, nil
}

// parseUnifiedDiff parses a byte-bounded `git diff` patch into whole-file
// DiffFiles. It stops before entering a file that cannot fit the file or row
// budget, so truncation never retains or returns a partial file. Path comes
// from the +++ (new) / --- (old, for deletes) headers; status comes from the
// extended headers, and added/removed are counted from retained rows.
func parseUnifiedDiff(patch []byte, maxFiles, maxRows int) ([]DiffFile, bool, error) {
	if maxFiles <= 0 || maxRows <= 0 {
		return nil, false, fmt.Errorf("%w: diff projection requires positive limits", ErrResultTooLarge)
	}
	parser := unifiedDiffParser{maxFiles: maxFiles, maxRows: maxRows}
	for encoded := range bytes.SplitSeq(patch, []byte{'\n'}) {
		stop, err := parser.consume(string(encoded))
		if err != nil {
			return nil, false, err
		}
		if stop {
			return parser.files, true, nil
		}
	}
	flushed, err := parser.flush()
	if err != nil {
		return nil, false, err
	}
	if !flushed {
		return parser.files, true, nil
	}
	return parser.files, false, nil
}

const (
	diffFileHeader        = "diff --git "
	newFileModeHeader     = "new file mode"
	deletedFileModeHeader = "deleted file mode"
	renameFromHeader      = "rename from "
	renameToHeader        = "rename to "
	binaryFilesHeader     = "Binary files "
	oldFileHeader         = "--- "
	newFileHeader         = "+++ "
	hunkHeader            = "@@"
	oldPathPrefix         = "a/"
	newPathPrefix         = "b/"
	nullPatchPath         = "/dev/null"
)

type unifiedDiffParser struct {
	maxFiles int
	maxRows  int
	files    []DiffFile
	current  *DiffFile
	left     hunkRange
	right    hunkRange
	rows     int
}

func (parser *unifiedDiffParser) consume(line string) (bool, error) {
	if line == `\ No newline at end of file` {
		if parser.current == nil || len(parser.current.Rows) == 0 {
			return false, fmt.Errorf("git: newline marker without a code row")
		}
		row := &parser.current.Rows[len(parser.current.Rows)-1]
		if row.Type == RowHunk || row.NoNewline ||
			row.LeftLine > 0 && parser.left.remaining != 0 || row.RightLine > 0 && parser.right.remaining != 0 {
			return false, fmt.Errorf("git: newline marker without a terminal code row")
		}
		row.NoNewline = true
		return false, nil
	}
	if parser.left.remaining > 0 || parser.right.remaining > 0 {
		return parser.appendRow(line)
	}
	if strings.HasPrefix(line, diffFileHeader) {
		return parser.startFile(strings.TrimPrefix(line, diffFileHeader))
	}
	if parser.current == nil {
		return false, nil
	}
	if parser.applyMetadata(line) {
		return false, nil
	}
	if strings.HasPrefix(line, hunkHeader) {
		return parser.appendHunk(line)
	}
	return false, nil
}

func (parser *unifiedDiffParser) startFile(header string) (bool, error) {
	flushed, err := parser.flush()
	if err != nil || !flushed || len(parser.files) >= parser.maxFiles {
		return true, err
	}
	parser.current = &DiffFile{
		Path:   diffHeaderPath(header),
		Status: StatusModified,
	}
	parser.left = hunkRange{}
	parser.right = hunkRange{}
	return false, nil
}

func (parser *unifiedDiffParser) flush() (bool, error) {
	if parser.left.remaining != 0 || parser.right.remaining != 0 {
		return false, fmt.Errorf("git: incomplete diff hunk")
	}
	if parser.current == nil {
		return true, nil
	}
	if len(parser.files) >= parser.maxFiles || parser.rows+len(parser.current.Rows) > parser.maxRows {
		return false, nil
	}
	parser.rows += len(parser.current.Rows)
	parser.files = append(parser.files, *parser.current)
	parser.current = nil
	return true, nil
}

func (parser *unifiedDiffParser) applyMetadata(line string) bool {
	switch {
	case strings.HasPrefix(line, newFileModeHeader):
		parser.current.Status = StatusAdded
	case strings.HasPrefix(line, deletedFileModeHeader):
		parser.current.Status = StatusDeleted
	case strings.HasPrefix(line, renameFromHeader):
		parser.current.PreviousPath = parsePatchPath(strings.TrimPrefix(line, renameFromHeader), "")
		parser.current.Status = StatusRenamed
	case strings.HasPrefix(line, renameToHeader):
		parser.current.Path = parsePatchPath(strings.TrimPrefix(line, renameToHeader), "")
		parser.current.Status = StatusRenamed
	case strings.HasPrefix(line, binaryFilesHeader):
		parser.current.Binary = true
		if path := binaryPatchPath(line); path != "" {
			parser.current.Path = path
		}
	case strings.HasPrefix(line, oldFileHeader):
		path := parsePatchPath(strings.TrimPrefix(line, oldFileHeader), oldPathPrefix)
		if parser.current.Path == "" && path != nullPatchPath {
			parser.current.Path = path
		}
	case strings.HasPrefix(line, newFileHeader):
		path := parsePatchPath(strings.TrimPrefix(line, newFileHeader), newPathPrefix)
		if path != nullPatchPath {
			parser.current.Path = path
		}
	default:
		return false
	}
	return true
}

func (parser *unifiedDiffParser) appendHunk(line string) (bool, error) {
	if parser.rowBudgetExhausted() {
		return true, nil
	}
	left, right, err := parseHunkHeader(line)
	if err != nil {
		return false, err
	}
	parser.left = left
	parser.right = right
	parser.current.Rows = append(parser.current.Rows, Row{Type: RowHunk, Text: line})
	return false, nil
}

func (parser *unifiedDiffParser) appendRow(line string) (bool, error) {
	if line == "" {
		return false, fmt.Errorf("git: incomplete diff hunk")
	}
	marker := line[0]
	left := marker == '-' || marker == ' '
	right := marker == '+' || marker == ' '
	if !left && !right || left && parser.left.remaining == 0 || right && parser.right.remaining == 0 {
		return false, fmt.Errorf("git: row exceeds diff hunk range: %q", line)
	}
	if parser.rowBudgetExhausted() {
		return true, nil
	}
	row := Row{Code: line[1:]}
	switch marker {
	case '+':
		row.Type = RowAdded
		parser.current.Added++
	case '-':
		row.Type = RowDeleted
		parser.current.Removed++
	case ' ':
		row.Type = RowContext
	}
	if left {
		row.LeftLine = parser.left.line
		parser.left.line++
		parser.left.remaining--
	}
	if right {
		row.RightLine = parser.right.line
		parser.right.line++
		parser.right.remaining--
	}
	parser.current.Rows = append(parser.current.Rows, row)
	return false, nil
}

func (parser *unifiedDiffParser) rowBudgetExhausted() bool {
	return parser.rows+len(parser.current.Rows) >= parser.maxRows
}

func parsePatchPath(value, prefix string) string {
	if strings.HasPrefix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return strings.TrimPrefix(value, prefix)
}

func diffHeaderPath(value string) string {
	if strings.HasPrefix(value, "\"") {
		_, remainder, ok := cutQuotedPath(value)
		if !ok {
			return ""
		}
		path, _, ok := cutQuotedPath(strings.TrimSpace(remainder))
		if !ok {
			return ""
		}
		return strings.TrimPrefix(path, "b/")
	}
	_, path, found := strings.Cut(value, " b/")
	if !found {
		return ""
	}
	return path
}

func cutQuotedPath(value string) (string, string, bool) {
	for index := 1; index < len(value); index++ {
		if value[index] != '"' {
			continue
		}
		backslashes := 0
		for cursor := index - 1; cursor >= 0 && value[cursor] == '\\'; cursor-- {
			backslashes++
		}
		if backslashes%2 != 0 {
			continue
		}
		unquoted, err := strconv.Unquote(value[:index+1])
		return unquoted, value[index+1:], err == nil
	}
	return "", value, false
}

func binaryPatchPath(line string) string {
	const marker = " and b/"
	index := strings.LastIndex(line, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSuffix(line[index+len(marker):], " differ")
}

type hunkRange struct {
	line      int
	remaining int
}

func parseHunkHeader(h string) (left, right hunkRange, err error) {
	fields := strings.Fields(h)
	if len(fields) < 4 || fields[0] != "@@" || fields[3] != "@@" ||
		len(fields[1]) < 2 || fields[1][0] != '-' ||
		len(fields[2]) < 2 || fields[2][0] != '+' {
		return left, right, fmt.Errorf("git: malformed hunk header %q", h)
	}
	left, err = parseHunkRange(fields[1][1:])
	if err != nil {
		return left, right, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	right, err = parseHunkRange(fields[2][1:])
	if err != nil {
		return left, right, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	return left, right, nil
}

func parseHunkRange(value string) (hunkRange, error) {
	start, count, explicit := strings.Cut(value, ",")
	line, err := strconv.Atoi(start)
	if err != nil || line < 0 {
		return hunkRange{}, fmt.Errorf("invalid line number %q", start)
	}
	remaining := 1
	if explicit {
		remaining, err = strconv.Atoi(count)
		if err != nil || remaining < 0 {
			return hunkRange{}, fmt.Errorf("invalid line count %q", count)
		}
	}
	return hunkRange{line: line, remaining: remaining}, nil
}
