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

func parseUnifiedDiff(summary comparisonSummary, maxRows int, retainRows bool) (parsedDiff, error) {
	parser := unifiedDiffParser{summary: summary, maxRows: maxRows, retainRows: retainRows}
	offset := 0
	for encoded := range bytes.SplitAfterSeq(summary.patch, []byte{'\n'}) {
		stop, err := parser.consume(bytes.TrimSuffix(encoded, []byte{'\n'}), offset)
		if err != nil {
			return parsedDiff{}, err
		}
		if stop {
			return parsedDiff{files: parser.files, end: parser.acceptedEnd, truncated: true}, nil
		}
		offset += len(encoded)
	}
	if err := parser.flush(len(summary.patch)); err != nil {
		return parsedDiff{}, err
	}
	if len(parser.files) != len(summary.files) {
		return parsedDiff{}, fmt.Errorf("git: patch omitted a reported file")
	}
	return parsedDiff{files: parser.files, end: parser.acceptedEnd, truncated: summary.truncated}, nil
}

const (
	diffFileHeader  = "diff --git "
	hunkHeader      = "@@"
	noNewlineMarker = `\ No newline at end of file`
)

const (
	addedRowMarker   = '+'
	deletedRowMarker = '-'
	contextRowMarker = ' '
)

type unifiedDiffParser struct {
	summary         comparisonSummary
	maxRows         int
	files           []DiffFile
	current         *DiffFile
	awaitingTypeAdd bool
	left            hunkRange
	right           hunkRange
	rows            int
	currentRows     int
	retainRows      bool
	lastRow         *Row
	scratch         Row
	acceptedEnd     int
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
		return parser.startFragment(offset)
	}
	if parser.current != nil && strings.HasPrefix(line, hunkHeader) {
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

func (parser *unifiedDiffParser) startFragment(offset int) (bool, error) {
	parser.lastRow = nil
	// With external diffs disabled, Git emits a type change as a deletion followed by an addition.
	if parser.awaitingTypeAdd {
		parser.awaitingTypeAdd = false
		return false, nil
	}
	if err := parser.flush(offset); err != nil {
		return false, err
	}
	if len(parser.files) == len(parser.summary.files) {
		if parser.summary.truncated {
			return true, nil
		}
		return false, fmt.Errorf("git: patch contains an unreported file")
	}
	file := parser.summary.files[len(parser.files)]
	parser.current = &DiffFile{FileChange: file.FileChange}
	parser.awaitingTypeAdd = file.typeChanged
	parser.currentRows = 0
	return false, nil
}

func (parser *unifiedDiffParser) flush(end int) error {
	if parser.left.remaining != 0 || parser.right.remaining != 0 {
		return fmt.Errorf("git: incomplete diff hunk")
	}
	if parser.awaitingTypeAdd {
		return fmt.Errorf("git: incomplete file type change")
	}
	if parser.current == nil {
		return nil
	}
	parser.rows += parser.currentRows
	parser.files = append(parser.files, *parser.current)
	parser.current = nil
	parser.acceptedEnd = end
	return nil
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
