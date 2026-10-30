package builtin

type CommandResult struct {
	Output   string `json:"output"`
	ExitCode *int   `json:"exitCode,omitzero"`
}

type SearchResult struct {
	Hits []SearchHit `json:"hits"`
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
