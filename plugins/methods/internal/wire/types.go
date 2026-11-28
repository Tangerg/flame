package wire

import "github.com/Tangerg/flame/packages/go-sdk/instant"

const (
	BoardMethod   = "flame.methods/board.v1"
	ReadMethod    = "flame.methods/execution.v1"
	SaveMethod    = "flame.methods/save.v1"
	StartMethod   = "flame.methods/start.v1"
	ApproveMethod = "flame.methods/approve.v1"
)

type ProjectInput struct {
	ProjectID string `json:"projectId"`
}

type Recipe struct {
	Title    string `json:"title"`
	Change   string `json:"change"`
	ReviewA  string `json:"reviewA"`
	ReviewB  string `json:"reviewB"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type SaveInput struct {
	ProjectInput
	Recipe
}

type Version struct {
	Recipe
	ID string `json:"id"`
}

type StartInput struct {
	ProjectInput
	VersionID  string `json:"versionId"`
	WorktreeID string `json:"worktreeId"`
}

type ExecutionInput struct {
	ProjectInput
	ID string `json:"id"`
}

type ApproveInput struct {
	ExecutionInput
	Digest   string `json:"digest"`
	Decision string `json:"decision,omitempty"`
}

type State string

const (
	Queued    State = "queued"
	Changing  State = "changing"
	Reviewing State = "reviewing"
	Awaiting  State = "awaiting_approval"
	Approved  State = "approved"
	Blocked   State = "blocked"
)

type Run struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
}

type Review struct {
	Slot   string `json:"slot"`
	RunID  string `json:"runId"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

type Decision struct {
	Disposition string            `json:"disposition,omitempty"`
	Digest      string            `json:"digest"`
	At          instant.Timestamp `json:"at"`
}

type Execution struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	Version    Version   `json:"version"`
	Workspace  string    `json:"workspace"`
	WorktreeID string    `json:"worktreeId"`
	Base       string    `json:"base"`
	State      State     `json:"state"`
	Reason     string    `json:"reason,omitempty"`
	Change     *Run      `json:"change,omitzero"`
	Review     *Run      `json:"review,omitzero"`
	Checks     []Review  `json:"checks"`
	Handoff    string    `json:"handoff,omitempty"`
	Patch      string    `json:"patch,omitempty"`
	Digest     string    `json:"digest,omitempty"`
	Decision   *Decision `json:"decision,omitzero"`
}

type Board struct {
	Latest     *Version    `json:"latest,omitzero"`
	Executions []Execution `json:"executions"`
}
