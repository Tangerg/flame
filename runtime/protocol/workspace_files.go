package protocol

import "time"

// WorkspaceQuery is the common explicit scope for workspace reads.
type WorkspaceQuery struct {
	Workspace WorkspaceRef `json:"workspace"`
}

// GetFileHeadRequest — workspace.files.head body.
type GetFileHeadRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path"`
	Lines     *int         `json:"lines,omitempty"`
}

// GrepRequest — workspace.files.search body. Query is a Go/RE2-compatible
// regular expression of at most 64 KiB. A zero Limit selects 100 retained
// matches; larger values are capped at 1000. The service may retain fewer rows
// when the 8 MiB result-material budget is reached while still reporting the
// exact Total for its admitted text corpus.
type GrepRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Query     string       `json:"query"`
	Path      string       `json:"path,omitempty"`
	Limit     *int         `json:"limit,omitempty"`
}

// ListFilesRequest is the workspace.files.list body. It lists files under
// Path (relative to CWD, jailed). Recursive (or a Glob) yields a flat subtree
// file list — the @file / fuzzy source; otherwise the immediate children — the
// lazy file-tree level. .gitignore + backstop excludes apply unless
// IncludeIgnored. PageQuery carries stable cursor pagination.
type ListFilesRequest struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Path           string       `json:"path,omitempty"`
	Glob           string       `json:"glob,omitempty"`
	Recursive      bool         `json:"recursive,omitempty"`
	IncludeIgnored bool         `json:"includeIgnored,omitempty"`
	PageQuery
}

// ReadFileRequest is the workspace.files.read body. It reads the whole
// file, or the StartLine..EndLine window (1-based inclusive, editor-facing)
// when given. A zero MaxBytes selects the 1 MiB default; larger values are
// capped at 8 MiB. FileContent.Truncated reports omitted source material.
// Non-regular files and content that is not valid UTF-8 text are unsupported.
type ReadFileRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path"`
	StartLine *int         `json:"startLine,omitempty"`
	EndLine   *int         `json:"endLine,omitempty"`
	MaxBytes  *int         `json:"maxBytes,omitempty"`
}

// FileContent is the workspace.files.read result. Content is valid UTF-8 text;
// binary files are rejected before projection. TotalLines is the whole-file
// line count even for a windowed read (so the UI can show "12–40 / 320").
// StartLine/EndLine describe the served window (1-based inclusive) and are
// present together when a range was requested and at least one line was served.
// A byte-limited last line may be a valid UTF-8 prefix of that source line; when
// even its first code point cannot fit, both window fields are omitted and
// Truncated remains true.
type FileContent struct {
	Content    string `json:"content"`
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	EndLine    int    `json:"endLine,omitempty"`
}

// FileEntryType is a listed workspace entry's kind.
type FileEntryType string

const (
	FileEntryFile    FileEntryType = "file"
	FileEntryDir     FileEntryType = "dir"
	FileEntrySymlink FileEntryType = "symlink"
)

// FileEntry is one inspected entry in workspace.files.list. Path
// is relative to the workspace root; type, size, and modification time come
// from one inspection of that entry.
type FileEntry struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       FileEntryType `json:"type"`
	SizeBytes  *int64        `json:"sizeBytes,omitempty"`
	ModifiedAt time.Time     `json:"modifiedAt"`
}

// FileHead is a file preview.
type FileHead struct {
	Lines []FileLine `json:"lines"`
}

// FileLine is one plain-text preview line for client-side highlighting.
type FileLine struct {
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}

// GrepResult is the workspace.files.search result. Matches is a
// stable whole-line prefix; Total is the exact count across admitted UTF-8 text
// files and may exceed len(Matches) when count or material limits apply.
type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
	Total   int         `json:"total"`
}

// GrepMatch is one plain-text grep hit.
type GrepMatch struct {
	Path       string `json:"path"`
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}
