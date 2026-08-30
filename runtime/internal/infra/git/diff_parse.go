package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

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
	if !parser.flush() {
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
	maxFiles  int
	maxRows   int
	files     []DiffFile
	current   *DiffFile
	leftLine  int
	rightLine int
	rows      int
}

func (parser *unifiedDiffParser) consume(line string) (bool, error) {
	if strings.HasPrefix(line, diffFileHeader) {
		return parser.startFile(strings.TrimPrefix(line, diffFileHeader)), nil
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
	return parser.appendRow(line), nil
}

func (parser *unifiedDiffParser) startFile(header string) bool {
	if !parser.flush() || len(parser.files) >= parser.maxFiles {
		return true
	}
	parser.current = &DiffFile{
		Path:   diffHeaderPath(header),
		Status: StatusModified,
	}
	parser.leftLine = 0
	parser.rightLine = 0
	return false
}

func (parser *unifiedDiffParser) flush() bool {
	if parser.current == nil {
		return true
	}
	if len(parser.files) >= parser.maxFiles || parser.rows+len(parser.current.Rows) > parser.maxRows {
		return false
	}
	parser.rows += len(parser.current.Rows)
	parser.files = append(parser.files, *parser.current)
	parser.current = nil
	return true
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
	parser.leftLine = left
	parser.rightLine = right
	parser.current.Rows = append(parser.current.Rows, Row{Type: RowHunk, Text: line})
	return false, nil
}

func (parser *unifiedDiffParser) appendRow(line string) bool {
	if line == "" {
		return false
	}
	marker := line[0]
	if marker != '+' && marker != '-' && marker != ' ' {
		return false
	}
	if parser.rowBudgetExhausted() {
		return true
	}
	var row Row
	switch marker {
	case '+':
		row = Row{Type: RowAdded, RightLine: parser.rightLine, Code: line[1:]}
		parser.rightLine++
		parser.current.Added++
	case '-':
		row = Row{Type: RowDeleted, LeftLine: parser.leftLine, Code: line[1:]}
		parser.leftLine++
		parser.current.Removed++
	case ' ':
		row = Row{Type: RowContext, LeftLine: parser.leftLine, RightLine: parser.rightLine, Code: line[1:]}
		parser.leftLine++
		parser.rightLine++
	}
	parser.current.Rows = append(parser.current.Rows, row)
	return false
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

// parseHunkHeader pulls the left/right start lines out of "@@ -L,S +R,S @@ …".
func parseHunkHeader(h string) (left, right int, err error) {
	fields := strings.Fields(h)
	if len(fields) < 4 || fields[0] != "@@" || fields[3] != "@@" ||
		len(fields[1]) < 2 || fields[1][0] != '-' ||
		len(fields[2]) < 2 || fields[2][0] != '+' {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q", h)
	}
	left, err = atoiBeforeComma(fields[1][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	right, err = atoiBeforeComma(fields[2][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	return left, right, nil
}

func atoiBeforeComma(s string) (int, error) {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid line number %q", s)
	}
	return n, nil
}
