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
	Name       string
	Type       protocol.FileEntryType
	SizeBytes  *int64
	ModifiedAt time.Time
}

func (f FileEntry) Validate() error {
	switch {
	case strings.TrimSpace(f.Path) == "":
		return errors.New("file entry path is empty")
	case strings.TrimSpace(f.Name) == "":
		return errors.New("file entry name is empty")
	case f.Type != protocol.FileEntryFile && f.Type != protocol.FileEntryDir && f.Type != protocol.FileEntrySymlink:
		return fmt.Errorf("file entry type %q is invalid", f.Type)
	case f.SizeBytes != nil && *f.SizeBytes < 0:
		return errors.New("file entry size is negative")
	default:
		return nil
	}
}

type FileListing struct {
	Entries []FileEntry
}

func (f FileListing) Validate() error {
	paths := make(map[string]struct{}, len(f.Entries))
	for index, entry := range f.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("file entry %d: %w", index, err)
		}
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
	Path       string
	Content    string
	TotalLines int
	Truncated  bool
	StartLine  int
	EndLine    int
}

func (f FileContent) Validate() error {
	switch {
	case strings.TrimSpace(f.Path) == "":
		return errors.New("file content path is empty")
	}
	if err := (protocol.FileContent{
		Path: f.Path, Content: f.Content, TotalLines: f.TotalLines,
		Truncated: f.Truncated, StartLine: f.StartLine, EndLine: f.EndLine,
	}).ValidateWire(); err != nil {
		return fmt.Errorf("file content: %w", err)
	}
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

type FileLine struct {
	Number int
	Text   string
}

type FileHead struct {
	Path  string
	Lines []FileLine
}

func (f FileHead) Validate() error {
	if strings.TrimSpace(f.Path) == "" {
		return errors.New("file head path is empty")
	}
	previous := 0
	for index, line := range f.Lines {
		if line.Number <= previous {
			return fmt.Errorf("file head line %d is not strictly ordered", index)
		}
		previous = line.Number
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

type Match struct {
	Path string
	Line int
	Text string
}

type SearchResult struct {
	Matches []Match
	Total   int
}

func (s SearchResult) Validate() error {
	if s.Total < len(s.Matches) {
		return errors.New("workspace search total is smaller than its matches")
	}
	for index, match := range s.Matches {
		if strings.TrimSpace(match.Path) == "" || match.Line <= 0 {
			return fmt.Errorf("workspace search match %d is invalid", index)
		}
	}
	return nil
}
