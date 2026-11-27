package wire

import "github.com/Tangerg/flame/packages/go-sdk/instant"

type MaterialCitation struct {
	DocumentID string `json:"documentId" jsonschema:"minLength=1,maxLength=128"`
	Revision   int64  `json:"revision" jsonschema:"minimum=1"`
	Page       int    `json:"page" jsonschema:"minimum=1"`
	FirstLine  int    `json:"firstLine" jsonschema:"minimum=1"`
	LastLine   int    `json:"lastLine" jsonschema:"minimum=1"`
}

type ClaimReadInput struct {
	Project  string `json:"project" jsonschema:"minLength=1"`
	ID       string `json:"id" jsonschema:"minLength=1,maxLength=128"`
	Revision int64  `json:"revision,omitzero" jsonschema:"minimum=1"`
}

type ClaimAcceptInput struct {
	Project          string             `json:"project" jsonschema:"minLength=1"`
	ID               string             `json:"id" jsonschema:"minLength=1,maxLength=128"`
	ExpectedRevision int64              `json:"expectedRevision" jsonschema:"minimum=0"`
	Statement        string             `json:"statement" jsonschema:"minLength=1,maxLength=4096"`
	Reason           string             `json:"reason" jsonschema:"minLength=1,maxLength=4096"`
	Citations        []MaterialCitation `json:"citations" jsonschema:"minItems=1,maxItems=8"`
}

type ClaimState string

const (
	ClaimCurrent     ClaimState = "current"
	ClaimRecheck     ClaimState = "recheck"
	ClaimUnavailable ClaimState = "unavailable"
)

func (ClaimState) JSONSchemaEnum() []string {
	return []string{string(ClaimCurrent), string(ClaimRecheck), string(ClaimUnavailable)}
}

type ClaimSummary struct {
	ID        string     `json:"id"`
	Revision  int64      `json:"revision"`
	State     ClaimState `json:"state"`
	Statement string     `json:"statement,omitempty"`
}

type ClaimEvidence struct {
	MaterialCitation
	Title        string `json:"title"`
	Source       string `json:"source"`
	Excerpt      string `json:"excerpt"`
	NeedsRecheck bool   `json:"needsRecheck"`
}

type ClaimRead struct {
	ClaimSummary
	AcceptedAt instant.Timestamp `json:"acceptedAt"`
	Reason     string            `json:"reason,omitempty"`
	Evidence   []ClaimEvidence   `json:"evidence"`
}

type ClaimReceipt struct {
	ID         string            `json:"id"`
	Revision   int64             `json:"revision"`
	AcceptedAt instant.Timestamp `json:"acceptedAt"`
}
