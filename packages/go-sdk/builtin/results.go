package builtin

type CommandResult struct {
	Output   string             `json:"output"`
	ExitCode *int               `json:"exitCode,omitzero"`
	Page     *CommandOutputPage `json:"page,omitzero"`
}

// CommandOutputPage records the source byte range and original shell handle of
// one captured log page. It grants no authority to replay or restart a command.
type CommandOutputPage struct {
	ShellID    string `json:"shellId,omitempty"`
	Status     string `json:"status,omitempty"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"nextOffset"`
	HasMore    bool   `json:"hasMore"`
	Dropped    bool   `json:"dropped"`
}

type SearchResult struct {
	Hits      []SearchHit     `json:"hits"`
	Total     *int            `json:"total,omitzero"`
	Complete  *bool           `json:"complete,omitzero"`
	Truncated bool            `json:"truncated,omitzero"`
	Coverage  *SearchCoverage `json:"coverage,omitzero"`
}

type SearchCoverage struct {
	ScannedFiles   int   `json:"scannedFiles"`
	SkippedFiles   int   `json:"skippedFiles"`
	RemainingFiles int   `json:"remainingFiles"`
	SourceBytes    int64 `json:"sourceBytes"`
}

type SearchHit struct {
	Path       string `json:"path"`
	Snippet    string `json:"snippet,omitempty"`
	LineNumber int    `json:"lineNumber,omitzero"`
}

type PatchResult struct {
	Changes []AppliedChange `json:"changes"`
}

type ChangeStatus string

const (
	Added    ChangeStatus = "added"
	Deleted  ChangeStatus = "deleted"
	Modified ChangeStatus = "modified"
	Moved    ChangeStatus = "moved"
)

func (ChangeStatus) Values() []string {
	return []string{string(Added), string(Deleted), string(Modified), string(Moved)}
}

// AppliedChange describes the tool's applied mutation, independently of VCS state.
type AppliedChange struct {
	Path    string       `json:"path"`
	Status  ChangeStatus `json:"status"`
	From    string       `json:"from,omitempty"`
	Added   int          `json:"added"`
	Removed int          `json:"removed"`
}
