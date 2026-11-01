// Package mcpconnector defines the installed external-server directory and its OAuth workflow.
package mcpconnector

const Reference = "flame.external-mcp.v1"
const Export = "external-servers"

type Decision string

const (
	Applied     Decision = "applied"
	Conflict    Decision = "conflict"
	Unavailable Decision = "unavailable"
)

type Outcome string

const (
	Unconfirmed Outcome = "unconfirmed"
	Pending     Outcome = "pending"
	Succeeded   Outcome = "succeeded"
	Failed      Outcome = "failed"
	Canceled    Outcome = "canceled"
)
const AttemptRetentionSeconds = 600

type Entry struct {
	Revision         string `json:"revision,omitempty"`
	Name             string `json:"name"`
	LaunchGrant      string `json:"launchGrant"`
	Enabled          bool   `json:"enabled"`
	Description      string `json:"description"`
	HandshakeSeconds int64  `json:"handshakeSeconds"`
}

type Selector struct {
	Name string `json:"name,omitempty"`
}
type Directory struct {
	Entries []Entry `json:"entries"`
}

// A missing replacement removes the entry. A missing expected revision creates it.
type Change struct {
	Name        string `json:"name"`
	Expected    string `json:"expected,omitempty"`
	Replacement *Entry `json:"replacement,omitempty"`
}

type Start struct {
	Name      string `json:"name"`
	Grant     string `json:"grant"`
	Epoch     string `json:"epoch"`
	AttemptID string `json:"attemptId"`
}
type Attempt struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     Outcome `json:"status"`
	StartedAt  string  `json:"startedAt"`
	FinishedAt string  `json:"finishedAt,omitempty"`
}
type Recovery struct {
	Epoch string `json:"epoch"`
}
type Request struct {
	ID    string `json:"id"`
	Epoch string `json:"epoch"`
}
type Completion struct {
	ID     string  `json:"id"`
	Epoch  string  `json:"epoch"`
	Status Outcome `json:"status"`
}
type AttemptResult struct {
	Attempt *Attempt `json:"attempt,omitempty"`
}
