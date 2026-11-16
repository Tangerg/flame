package protocol

// DiffMode selects the baseline workspace.diff.get compares against.
//
//flame:enum
type DiffMode string

const (
	DiffModeWorktree DiffMode = "worktree" // changes vs HEAD, incl. untracked (default)
	DiffModeBase     DiffMode = "base"     // vs merge-base with the default branch
)

// DiffFormat selects the workspace.diff.get result shape.
//
//flame:enum
type DiffFormat string

const (
	DiffFormatRows   DiffFormat = "rows"   // per-file structured diff (default)
	DiffFormatRaw    DiffFormat = "raw"    // unified patch string
	DiffFormatVisual DiffFormat = "visual" // bounded patch and its same-read manifest
)

// GetDiffRequest is the workspace.diff.get body. Mode selects the
// baseline (worktree=changes vs HEAD incl. untracked; base=vs merge-base with
// default branch). Format selects rows, complete raw text, or a visual patch
// with authoritative metadata. Limit caps rows in either bounded projection;
// exceeding it sets the projection's Truncated flag at a whole-file boundary.
type GetDiffRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path,omitempty"`
	Mode      DiffMode     `json:"mode,omitempty"` // "worktree" (default) | "base"
	Format    DiffFormat   `json:"format,omitempty"`
	Limit     *int         `json:"limit,omitzero"`
}

// DiffBaseline names the exact comparison source. An unborn repository compares
// against the empty tree; head and mergeBase carry resolved commit identities.
//
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

// Diff explicitly identifies its read projection. Visual patch bytes and the
// manifest belong to one accepted read; they cannot be combined with another
// response or interpreted as the complete raw projection.
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

// VisualDiff contains only complete files within Budget. An empty manifest
// with Truncated set means no complete file fit, not a clean comparison.
type VisualDiff struct {
	Patch     string                `json:"patch"`
	Manifest  []WorkspaceFileChange `json:"manifest"`
	Truncated bool                  `json:"truncated"`
	Budget    DiffBudget            `json:"budget"`
}

// FileStatus is the past-tense working-tree status vocabulary shared by
// WorkspaceFileChange and FileDiff.
//
//flame:enum
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileDiff is one file's structured diff. Added/Removed are
// omitted for a Binary file (Rows empty) rather than reported as a fake 0;
// PreviousPath is set only for renames.
type FileDiff struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitzero"`
	Removed      *int       `json:"removed,omitzero"`
	Binary       bool       `json:"binary,omitzero"`
	Rows         []DiffRow  `json:"rows"`
}

// WorkspaceFileChange is one entry in workspace.changes.list, whose complete
// result is ordered by Path ascending.
// Added/Removed are omitted for a Binary file (not a fake 0); PreviousPath is
// set only for renames.
type WorkspaceFileChange struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitzero"`
	Removed      *int       `json:"removed,omitzero"`
	Binary       bool       `json:"binary,omitzero"`
}
