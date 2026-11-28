package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type parsedDiff struct {
	files     []DiffFile
	end       int
	truncated bool
}

func parseUnifiedDiff(patch []byte, maxFiles, maxRows int, retainRows bool) (parsedDiff, error) {
	if maxFiles <= 0 || maxRows <= 0 {
		return parsedDiff{}, fmt.Errorf("%w: diff projection requires positive limits", ErrResultTooLarge)
	}
	parser := unifiedDiffParser{maxFiles: maxFiles, maxRows: maxRows, retainRows: retainRows}
	offset := 0
	for encoded := range bytes.SplitAfterSeq(patch, []byte{'\n'}) {
		stop, err := parser.consume(bytes.TrimSuffix(encoded, []byte{'\n'}), offset)
		if err != nil {
			return parsedDiff{}, err
		}
		if stop {
			return parsedDiff{files: parser.files, end: parser.acceptedEnd, truncated: true}, nil
		}
		offset += len(encoded)
	}
	flushed, err := parser.flush(len(patch))
	if err != nil {
		return parsedDiff{}, err
	}
	return parsedDiff{files: parser.files, end: parser.acceptedEnd, truncated: !flushed}, nil
}

const (
	diffFileHeader        = "diff --git "
	newFileModeHeader     = "new file mode"
	deletedFileModeHeader = "deleted file mode"
	renameFromHeader      = "rename from "
	renameToHeader        = "rename to "
	binaryFilesHeader     = "Binary files "
	hunkHeader            = "@@"
	oldPathPrefix         = "a/"
	newPathPrefix         = "b/"
	noNewlineMarker       = `\ No newline at end of file`
)

const (
	addedRowMarker   = '+'
	deletedRowMarker = '-'
	contextRowMarker = ' '
)

type unifiedDiffParser struct {
	maxFiles    int
	maxRows     int
	files       []DiffFile
	current     *DiffFile
	left        hunkRange
	right       hunkRange
	rows        int
	currentRows int
	retainRows  bool
	lastRow     *Row
	scratch     Row
	acceptedEnd int
}

func (parser *unifiedDiffParser) consume(encoded []byte, offset int) (bool, error) {
	if string(encoded) == noNewlineMarker {
		return false, parser.markNoNewline()
	}
	if parser.left.remaining > 0 || parser.right.remaining > 0 {
		return parser.appendRow(encoded)
	}
	line := string(encoded)
	if strings.HasPrefix(line, diffFileHeader) {
		return parser.startFragment(strings.TrimPrefix(line, diffFileHeader), offset)
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

func (parser *unifiedDiffParser) markNoNewline() error {
	if parser.current == nil || parser.lastRow == nil {
		return fmt.Errorf("git: newline marker without a code row")
	}
	row := parser.lastRow
	if row.Type == RowHunk || row.NoNewline ||
		row.LeftLine > 0 && parser.left.remaining != 0 || row.RightLine > 0 && parser.right.remaining != 0 {
		return fmt.Errorf("git: newline marker without a terminal code row")
	}
	row.NoNewline = true
	return nil
}

func (parser *unifiedDiffParser) startFragment(header string, offset int) (bool, error) {
	path := sameFilePatchPath(header)
	// Git emits both sides of a type change as consecutive fragments of the same file.
	if path != "" && parser.current != nil && path == parser.current.Path {
		parser.lastRow = nil
		return false, nil
	}
	flushed, err := parser.flush(offset)
	if err != nil || !flushed || len(parser.files) >= parser.maxFiles {
		return true, err
	}
	parser.current = &DiffFile{
		Path:   path,
		Status: StatusModified,
	}
	parser.left = hunkRange{}
	parser.right = hunkRange{}
	parser.currentRows = 0
	parser.lastRow = nil
	return false, nil
}

func (parser *unifiedDiffParser) flush(end int) (bool, error) {
	if parser.left.remaining != 0 || parser.right.remaining != 0 {
		return false, fmt.Errorf("git: incomplete diff hunk")
	}
	if parser.current == nil {
		return true, nil
	}
	if len(parser.files) >= parser.maxFiles || parser.rows+parser.currentRows > parser.maxRows {
		return false, nil
	}
	if parser.current.Path == "" {
		return false, fmt.Errorf("git: patch file has no path")
	}
	parser.rows += parser.currentRows
	parser.files = append(parser.files, *parser.current)
	parser.current = nil
	parser.acceptedEnd = end
	return true, nil
}

func (parser *unifiedDiffParser) applyMetadata(line string) bool {
	switch {
	case strings.HasPrefix(line, newFileModeHeader):
		if parser.current.Status == StatusDeleted {
			parser.current.Status = StatusModified
		} else {
			parser.current.Status = StatusAdded
		}
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
	parser.recordRow(Row{Type: RowHunk, Text: line})
	return false, nil
}

func (parser *unifiedDiffParser) appendRow(line []byte) (bool, error) {
	if len(line) == 0 {
		return false, fmt.Errorf("git: incomplete diff hunk")
	}
	rowType, left, right := codeRowSides(line[0])
	if !left && !right || left && parser.left.remaining == 0 || right && parser.right.remaining == 0 {
		return false, fmt.Errorf("git: row exceeds diff hunk range: %q", line)
	}
	if parser.rowBudgetExhausted() {
		return true, nil
	}
	row := Row{Type: rowType}
	if parser.retainRows {
		row.Code = string(line[1:])
	}
	switch rowType {
	case RowAdded:
		parser.current.Added++
	case RowDeleted:
		parser.current.Removed++
	case RowHunk, RowContext:
	}
	if left {
		row.LeftLine = parser.left.advance()
	}
	if right {
		row.RightLine = parser.right.advance()
	}
	parser.recordRow(row)
	return false, nil
}

func codeRowSides(marker byte) (rowType RowType, left, right bool) {
	switch marker {
	case addedRowMarker:
		return RowAdded, false, true
	case deletedRowMarker:
		return RowDeleted, true, false
	case contextRowMarker:
		return RowContext, true, true
	default:
		return "", false, false
	}
}

func (parser *unifiedDiffParser) recordRow(row Row) {
	parser.currentRows++
	if parser.retainRows {
		parser.current.Rows = append(parser.current.Rows, row)
		parser.lastRow = &parser.current.Rows[len(parser.current.Rows)-1]
		return
	}
	parser.scratch = row
	parser.lastRow = &parser.scratch
}

func (parser *unifiedDiffParser) rowBudgetExhausted() bool {
	return parser.rows+parser.currentRows >= parser.maxRows
}

func parsePatchPath(value, prefix string) string {
	if strings.HasPrefix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return strings.TrimPrefix(value, prefix)
}

func sameFilePatchPath(header string) string {
	// Git repeats the path with equal-width prefixes, even when it contains spaces.
	separator := len(header) / 2
	if len(header)%2 == 0 || header[separator] != ' ' {
		return ""
	}
	before := parsePatchPath(header[:separator], oldPathPrefix)
	after := parsePatchPath(header[separator+1:], newPathPrefix)
	if before != after {
		return ""
	}
	return after
}

type hunkRange struct {
	line      int
	remaining int
}

func (r *hunkRange) advance() int {
	line := r.line
	r.line++
	r.remaining--
	return line
}

func parseHunkHeader(h string) (left, right hunkRange, err error) {
	fields := strings.Fields(h)
	if len(fields) < 4 || fields[0] != hunkHeader || fields[3] != hunkHeader ||
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
