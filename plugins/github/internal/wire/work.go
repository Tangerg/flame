package wire

import "github.com/Tangerg/flame/packages/go-sdk/instant"

type ReviewWorkInput struct {
	IssueInput
	Revision string `json:"revision,omitempty" jsonschema:"maxLength=129"`
}

type ReviewFindingInput struct {
	Path    string   `json:"path" jsonschema:"minLength=1,maxLength=4096"`
	Side    DiffSide `json:"side"`
	Line    int64    `json:"line" jsonschema:"minimum=1"`
	Summary string   `json:"summary" jsonschema:"minLength=1,maxLength=2048"`
}

type FindingDisposition string

const (
	FindingAccepted FindingDisposition = "accepted"
	FindingDeferred FindingDisposition = "deferred"
	FindingResolved FindingDisposition = "resolved"
)

func (FindingDisposition) JSONSchemaEnum() []string {
	return []string{string(FindingAccepted), string(FindingDeferred), string(FindingResolved)}
}

type FindingDecisionInput struct {
	Finding     string             `json:"finding" jsonschema:"minLength=1,maxLength=128"`
	Disposition FindingDisposition `json:"disposition"`
	Reason      string             `json:"reason" jsonschema:"minLength=1,maxLength=2048"`
}

type ReviewWorkChange struct {
	ReviewWorkInput
	ExpectedVersion int64                 `json:"expectedVersion" jsonschema:"minimum=0"`
	Finding         *ReviewFindingInput   `json:"finding,omitzero"`
	Decision        *FindingDecisionInput `json:"decision,omitzero"`
	Follow          *bool                 `json:"follow,omitzero"`
}

type FindingDecision struct {
	Disposition FindingDisposition `json:"disposition"`
	Reason      string             `json:"reason"`
	At          instant.Timestamp  `json:"at"`
}

type ReviewFinding struct {
	ReviewFindingInput
	ID        string            `json:"id"`
	Decisions []FindingDecision `json:"decisions"`
}

type ReviewVersion struct {
	ID             string         `json:"id"`
	BaseSHA        string         `json:"baseSha"`
	HeadSHA        string         `json:"headSha"`
	HeadRepository string         `json:"headRepository,omitempty"`
	Title          string         `json:"title"`
	Recheck        *ReviewRecheck `json:"recheck,omitzero"`
}

type ReviewRecheckStatus string

const (
	ReviewRecheckPending ReviewRecheckStatus = "pending"
	ReviewRecheckClean   ReviewRecheckStatus = "clean"
	ReviewRecheckIssues  ReviewRecheckStatus = "issues"
	ReviewRecheckFailed  ReviewRecheckStatus = "failed"
)

func (ReviewRecheckStatus) JSONSchemaEnum() []string {
	return []string{string(ReviewRecheckPending), string(ReviewRecheckClean), string(ReviewRecheckIssues), string(ReviewRecheckFailed)}
}

type FindingRecheckStatus string

const (
	FindingFileUnchanged   FindingRecheckStatus = "unchanged"
	FindingFileChanged     FindingRecheckStatus = "changed"
	FindingFileUnavailable FindingRecheckStatus = "unavailable"
)

func (FindingRecheckStatus) JSONSchemaEnum() []string {
	return []string{string(FindingFileUnchanged), string(FindingFileChanged), string(FindingFileUnavailable)}
}

type FindingRecheck struct {
	Finding        string               `json:"finding"`
	Status         FindingRecheckStatus `json:"status"`
	SourceRevision string               `json:"sourceRevision,omitempty"`
	Patch          string               `json:"patch,omitempty"`
	PatchTruncated bool                 `json:"patchTruncated,omitempty"`
	Decisions      []FindingDecision    `json:"decisions,omitempty"`
}

type ProjectCheckStatus string

const (
	ProjectCheckPending     ProjectCheckStatus = "pending"
	ProjectCheckPassed      ProjectCheckStatus = "passed"
	ProjectCheckFailed      ProjectCheckStatus = "failed"
	ProjectCheckTimedOut    ProjectCheckStatus = "timed_out"
	ProjectCheckUnavailable ProjectCheckStatus = "unavailable"
	ProjectCheckNotRun      ProjectCheckStatus = "not_run"
)

func (ProjectCheckStatus) JSONSchemaEnum() []string {
	return []string{string(ProjectCheckPending), string(ProjectCheckPassed), string(ProjectCheckFailed), string(ProjectCheckTimedOut), string(ProjectCheckUnavailable), string(ProjectCheckNotRun)}
}

type ProjectCheck struct {
	Name           string             `json:"name"`
	Command        string             `json:"command"`
	TimeoutSeconds int                `json:"timeoutSeconds" jsonschema:"minimum=1"`
	Status         ProjectCheckStatus `json:"status"`
	ExitCode       *int               `json:"exitCode,omitzero"`
	DurationMillis *int64             `json:"durationMillis,omitzero"`
	Log            string             `json:"log,omitempty"`
	LogTruncated   bool               `json:"logTruncated,omitempty"`
}

type ProjectVerification struct {
	Environment  string         `json:"environment"`
	PolicyDigest string         `json:"policyDigest"`
	Runner       string         `json:"runner"`
	Checks       []ProjectCheck `json:"checks"`
}

type ReviewRecheck struct {
	Status           ReviewRecheckStatus  `json:"status"`
	PreviousRevision string               `json:"previousRevision"`
	CheckedHead      string               `json:"checkedHead,omitempty"`
	CompletedAt      *instant.Timestamp   `json:"completedAt,omitzero"`
	Diagnostic       string               `json:"diagnostic,omitempty"`
	Findings         []FindingRecheck     `json:"findings"`
	FindingsPartial  bool                 `json:"findingsPartial,omitempty"`
	Project          *ProjectVerification `json:"project,omitzero"`
}

type ReviewCheck struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	HeadSHA    string  `json:"headSha"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion,omitzero"`
	URL        string  `json:"url"`
}

type ReviewDiscussion struct {
	Comment
	ReceivedAtRevision string `json:"receivedAtRevision"`
}

type ReviewWork struct {
	IssueInput
	URL           string             `json:"url"`
	Version       int64              `json:"version"`
	Latest        string             `json:"latest,omitempty"`
	CurrentHead   string             `json:"currentHead"`
	Following     bool               `json:"following"`
	FollowError   string             `json:"followError,omitempty"`
	Selected      *ReviewVersion     `json:"selected,omitzero"`
	Versions      []ReviewVersion    `json:"versions"`
	Checks        []ReviewCheck      `json:"checks"`
	ChecksPartial bool               `json:"checksPartial"`
	Findings      []ReviewFinding    `json:"findings"`
	NeedsRecheck  []ReviewFinding    `json:"needsRecheck"`
	Discussion    []ReviewDiscussion `json:"discussion"`
}
