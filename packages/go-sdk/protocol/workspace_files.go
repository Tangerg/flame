package protocol

import "time"

type WorkspaceQuery struct {
	Workspace WorkspaceRef `json:"workspace"`
}

type GrepRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Query     string       `json:"query"`
	Path      string       `json:"path,omitempty"`
	Limit     *int         `json:"limit,omitzero"`
	ScanAll   bool         `json:"scanAll,omitzero"`
}

type ListFilesRequest struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Path           string       `json:"path,omitempty"`
	Glob           string       `json:"glob,omitempty"`
	NameQuery      *string      `json:"nameQuery,omitzero"`
	Recursive      bool         `json:"recursive,omitzero"`
	IncludeIgnored bool         `json:"includeIgnored,omitzero"`
	PageQuery
}

type ReadFileRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path"`
	StartLine *int         `json:"startLine,omitzero"`
	EndLine   *int         `json:"endLine,omitzero"`
	MaxBytes  *int         `json:"maxBytes,omitzero"`
	FromEnd   bool         `json:"fromEnd,omitzero"`
}

type FileContent struct {
	Content    string `json:"content"`
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitzero"`
	StartLine  int    `json:"startLine,omitzero"`
	EndLine    int    `json:"endLine,omitzero"`
}

//flame:enum
type FileEntryType string

const (
	FileEntryFile    FileEntryType = "file"
	FileEntryDir     FileEntryType = "dir"
	FileEntrySymlink FileEntryType = "symlink"
)

type FileEntry struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       FileEntryType `json:"type"`
	SizeBytes  *int64        `json:"sizeBytes,omitzero"`
	ModifiedAt time.Time     `json:"modifiedAt"`
}

type GrepResult struct {
	Matches  []GrepMatch  `json:"matches"`
	Total    int          `json:"total"`
	Complete bool         `json:"complete"`
	Coverage GrepCoverage `json:"coverage"`
}

type GrepCoverage struct {
	ScannedFiles   int   `json:"scannedFiles"`
	SkippedFiles   int   `json:"skippedFiles"`
	RemainingFiles int   `json:"remainingFiles"`
	SourceBytes    int64 `json:"sourceBytes"`
}

type GrepMatch struct {
	Path       string `json:"path"`
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}

type FileListPage struct {
	Data []FileEntry `json:"data"`
	PageContinuation
	Complete bool             `json:"complete"`
	Coverage FileListCoverage `json:"coverage"`
}

type FileListCoverage struct {
	ScannedEntries  int  `json:"scannedEntries"`
	ExcludedEntries int  `json:"excludedEntries"`
	BudgetExhausted bool `json:"budgetExhausted"`
}
