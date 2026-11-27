package wire

import "github.com/Tangerg/flame/packages/go-sdk/instant"

type IssueDeliveriesInput struct {
	ProjectID string `json:"projectId" jsonschema:"minLength=1,maxLength=256"`
	Before    int64  `json:"before,omitempty" jsonschema:"minimum=0"`
}

type IssueDeliveryPage struct {
	Items []IssueDelivery `json:"items"`
	Next  int64           `json:"next,omitempty"`
}

type IssueDelivery struct {
	ID string `json:"id"`
	IssueInput
	Title      string                `json:"title"`
	IssueURL   string                `json:"issueUrl"`
	Branch     string                `json:"branch"`
	Head       string                `json:"head,omitempty"`
	Workspace  string                `json:"workspace,omitempty"`
	SessionID  string                `json:"sessionId,omitempty"`
	RunID      string                `json:"runId,omitempty"`
	Status     string                `json:"status"`
	Reason     string                `json:"reason,omitempty"`
	Project    *ProjectVerification  `json:"project,omitzero"`
	CI         *IssueVerificationCI  `json:"ci,omitzero"`
	Draft      string                `json:"draft,omitempty"`
	Digest     string                `json:"digest,omitempty"`
	ApprovedAt *instant.Timestamp    `json:"approvedAt,omitzero"`
	Receipt    *IssueDeliveryReceipt `json:"receipt,omitzero"`
}

type IssueVerificationCI struct {
	Status string     `json:"status"`
	Checks []CheckRun `json:"checks"`
}

type PublishIssueDeliveryInput struct {
	ID     string `json:"id" jsonschema:"pattern=^[0-9a-f]{64}$"`
	Digest string `json:"digest" jsonschema:"pattern=^[0-9a-f]{64}$"`
}

type IssueDeliveryReceipt struct {
	Head    string  `json:"head"`
	Digest  string  `json:"digest"`
	Comment Comment `json:"comment"`
}
