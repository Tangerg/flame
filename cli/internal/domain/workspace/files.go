package workspace

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

type FileEntry struct {
	Path       string
	Type       protocol.FileEntryType
	SizeBytes  *int64
	ModifiedAt time.Time
}

type FileListing struct {
	Entries []FileEntry
}

// Validate rejects a listing whose entries would collapse in the CLI. Every
// other entry rule belongs to the Runtime wire contract.
func (f FileListing) Validate() error {
	paths := make(map[string]struct{}, len(f.Entries))
	for index, entry := range f.Entries {
		if _, exists := paths[entry.Path]; exists {
			return fmt.Errorf("file entry %d repeats path %q", index, entry.Path)
		}
		paths[entry.Path] = struct{}{}
	}
	return nil
}

type FilesRequest struct {
	Workspace      string
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

func (f FilesRequest) Validate() error {
	if strings.TrimSpace(f.Workspace) == "" {
		return errors.New("file list workspace is empty")
	}
	return nil
}

type ReadRequest struct {
	Workspace string
	Path      string
	Range     ReadLineRange
	ByteLimit ReadByteLimit
}

func (r ReadRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.Workspace) == "":
		return errors.New("file read workspace is empty")
	case strings.TrimSpace(r.Path) == "":
		return errors.New("file read path is empty")
	default:
		if _, _, err := r.Range.Bounds(); err != nil {
			return err
		}
		_, err := r.ByteLimit.Bytes()
		return err
	}
}

type FileContent struct {
	Content    string
	TotalLines int
	Truncated  bool
	StartLine  int
	EndLine    int
}

// Validate checks the read window relationships the CLI renders with. The wire
// contract owns presence and positivity; it cannot compare the two bounds.
func (f FileContent) Validate() error {
	switch {
	case f.EndLine > 0 && f.EndLine < f.StartLine:
		return errors.New("file content window is reversed")
	case f.EndLine > f.TotalLines:
		return errors.New("file content window exceeds the file line count")
	default:
		return nil
	}
}

func (f FileContent) Window() string {
	if f.StartLine == 0 {
		return fmt.Sprintf("%d lines", f.TotalLines)
	}
	return fmt.Sprintf("lines %d-%d/%d", f.StartLine, f.EndLine, f.TotalLines)
}

type HeadRequest struct {
	Workspace string
	Path      string
	LineLimit HeadLineLimit
}

func (h HeadRequest) Validate() error {
	if strings.TrimSpace(h.Workspace) == "" || strings.TrimSpace(h.Path) == "" {
		return errors.New("file head requires workspace and path")
	}
	_, err := h.LineLimit.Lines()
	return err
}

type FileHead struct {
	Lines []protocol.FileLine
}

func (f FileHead) Validate() error {
	previous := 0
	for index, line := range f.Lines {
		if line.LineNumber <= previous {
			return fmt.Errorf("file head line %d is not strictly ordered", index)
		}
		previous = line.LineNumber
	}
	return nil
}

type SearchRequest struct {
	Workspace string
	Query     string
	Path      string
	Limit     SearchResultLimit
}

func (s SearchRequest) Validate() error {
	if strings.TrimSpace(s.Workspace) == "" || strings.TrimSpace(s.Query) == "" {
		return errors.New("workspace search requires workspace and query")
	}
	_, err := s.Limit.Matches()
	return err
}

type SearchResult struct {
	Matches []protocol.GrepMatch
	Total   int
}

func (s SearchResult) Validate() error {
	if s.Total < len(s.Matches) {
		return errors.New("workspace search total is smaller than its matches")
	}
	return nil
}
