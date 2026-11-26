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
)

func (FindingDisposition) JSONSchemaEnum() []string {
	return []string{string(FindingAccepted), string(FindingDeferred)}
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
	ID             string `json:"id"`
	BaseSHA        string `json:"baseSha"`
	HeadSHA        string `json:"headSha"`
	HeadRepository string `json:"headRepository,omitempty"`
	Title          string `json:"title"`
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
	Selected      *ReviewVersion     `json:"selected,omitzero"`
	Versions      []ReviewVersion    `json:"versions"`
	Checks        []ReviewCheck      `json:"checks"`
	ChecksPartial bool               `json:"checksPartial"`
	Findings      []ReviewFinding    `json:"findings"`
	NeedsRecheck  []ReviewFinding    `json:"needsRecheck"`
	Discussion    []ReviewDiscussion `json:"discussion"`
}
