package builtin

type CommandResult struct {
	Output   string             `json:"output"`
	ExitCode *int               `json:"exitCode,omitzero"`
	Page     *CommandOutputPage `json:"page,omitzero"`
}

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

type AppliedChange struct {
	Path    string       `json:"path"`
	Status  ChangeStatus `json:"status"`
	From    string       `json:"from,omitempty"`
	Added   int          `json:"added"`
	Removed int          `json:"removed"`
}

type ScriptResult struct {
	Status ScriptStatus `json:"status"`
	Error  string       `json:"error,omitempty"`
	Output string       `json:"output"`
	Calls  []ScriptCall `json:"calls"`
}

type ScriptStatus string

const (
	ScriptCompleted           ScriptStatus = "completed"
	ScriptFailed              ScriptStatus = "failed"
	ScriptTimedOut            ScriptStatus = "timedOut"
	ScriptCallLimitExceeded   ScriptStatus = "callLimitExceeded"
	ScriptOutputLimitExceeded ScriptStatus = "outputLimitExceeded"
	ScriptCanceled            ScriptStatus = "canceled"
)

func (ScriptStatus) Values() []string {
	return []string{
		string(ScriptCompleted), string(ScriptFailed), string(ScriptTimedOut),
		string(ScriptCallLimitExceeded), string(ScriptOutputLimitExceeded), string(ScriptCanceled),
	}
}

// ScriptCall records one Tool call a script requested. Source is the canonical
// Tool reference and is empty when the script named no callable Tool.
type ScriptCall struct {
	Tool      string           `json:"tool"`
	Source    string           `json:"source,omitempty"`
	Arguments map[string]any   `json:"arguments,omitzero"`
	Status    ScriptCallStatus `json:"status"`
	Error     string           `json:"error,omitempty"`
	Result    any              `json:"result,omitzero"`
	// OmittedBytes is the size of a result the record did not retain.
	OmittedBytes int `json:"omittedBytes,omitzero"`
}

type ScriptCallStatus string

const (
	ScriptCallSucceeded ScriptCallStatus = "succeeded"
	ScriptCallFailed    ScriptCallStatus = "failed"
	ScriptCallRejected  ScriptCallStatus = "rejected"
	// The call started but the script ended before its outcome arrived.
	ScriptCallUnconfirmed ScriptCallStatus = "unconfirmed"
	// The script requested the call but it never started.
	ScriptCallNotDispatched ScriptCallStatus = "notDispatched"
)

func (ScriptCallStatus) Values() []string {
	return []string{
		string(ScriptCallSucceeded), string(ScriptCallFailed), string(ScriptCallRejected),
		string(ScriptCallUnconfirmed), string(ScriptCallNotDispatched),
	}
}
