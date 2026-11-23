package protocol

//flame:enum
type DiffMode string

const (
	DiffModeWorktree DiffMode = "worktree"
	DiffModeBase     DiffMode = "base"
)

//flame:enum
type DiffFormat string

const (
	DiffFormatRows   DiffFormat = "rows"
	DiffFormatRaw    DiffFormat = "raw"
	DiffFormatVisual DiffFormat = "visual"
)

type GetDiffRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path,omitempty"`
	Mode      DiffMode     `json:"mode,omitempty"`
	Format    DiffFormat   `json:"format,omitempty"`
	Limit     *int         `json:"limit,omitzero"`
}

//flame:enum
type DiffBaselineType string

const (
	DiffBaselineHead      DiffBaselineType = "head"
	DiffBaselineMergeBase DiffBaselineType = "mergeBase"
	DiffBaselineEmptyTree DiffBaselineType = "emptyTree"
)

type DiffBaseline struct {
	Type   DiffBaselineType `json:"type"`
	Commit string           `json:"commit,omitempty"`
}

type Diff struct {
	Type     DiffFormat   `json:"type"`
	Baseline DiffBaseline `json:"baseline"`
	Rows     *DiffRows    `json:"rows,omitzero"`
	Patch    *string      `json:"patch,omitzero"`
	Visual   *VisualDiff  `json:"visual,omitzero"`
}

type DiffBudget struct {
	Files int `json:"files"`
	Rows  int `json:"rows"`
	Bytes int `json:"bytes"`
}

type DiffRows struct {
	Files     []FileDiff `json:"files"`
	Truncated bool       `json:"truncated"`
	Budget    DiffBudget `json:"budget"`
}

type VisualDiff struct {
	Patch     string                `json:"patch"`
	Manifest  []WorkspaceFileChange `json:"manifest"`
	Truncated bool                  `json:"truncated"`
	Budget    DiffBudget            `json:"budget"`
}

//flame:enum
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

type FileDiff struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"`
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitzero"`
	Removed      *int       `json:"removed,omitzero"`
	Binary       bool       `json:"binary,omitzero"`
	Rows         []DiffRow  `json:"rows"`
}

type WorkspaceFileChange struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"`
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitzero"`
	Removed      *int       `json:"removed,omitzero"`
	Binary       bool       `json:"binary,omitzero"`
}
