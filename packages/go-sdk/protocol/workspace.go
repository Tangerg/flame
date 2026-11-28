package protocol

import (
	"time"
)

type WorkspaceRef struct {
	Path string `json:"path"`
}

//flame:enum
type WorkspaceAvailability string

const (
	WorkspaceAvailable WorkspaceAvailability = "available"
	WorkspaceMissing   WorkspaceAvailability = "missing"
)

type WorkspaceInfo struct {
	Ref            WorkspaceRef          `json:"ref"`
	RepositoryRoot string                `json:"repositoryRoot,omitempty"`
	Availability   WorkspaceAvailability `json:"availability"`
}

type WorkspaceSummary struct {
	Workspace    WorkspaceInfo `json:"workspace"`
	Name         string        `json:"name"`
	SessionCount int           `json:"sessionCount"`
	LastActiveAt *time.Time    `json:"lastActiveAt,omitzero"`
}

type ResolveWorkspaceRequest struct {
	Ref *WorkspaceRef `json:"ref,omitzero"`
}

type TaskWorktree struct {
	ID        string         `json:"id"`
	Workspace WorkspaceRef   `json:"workspace"`
	Source    WorkspaceRef   `json:"source"`
	Base      string         `json:"base"`
	Available bool           `json:"available"`
	Setup     *WorktreeSetup `json:"setup,omitzero"`
}

type WorktreeSetup struct {
	Command  string `json:"command"`
	ExitCode *int   `json:"exitCode,omitzero"`
	Output   string `json:"output"`
}

type CreateTaskWorktreeRequest struct {
	Source WorkspaceRef `json:"source"`
	Base   string       `json:"base"`
	Setup  string       `json:"setup,omitempty"`
}

type TaskWorktreeRequest struct {
	ID string `json:"id"`
}

type WorktreeHandoff struct {
	Worktree TaskWorktree `json:"worktree"`
	Branch   *string      `json:"branch,omitzero"`
	Head     string       `json:"head"`
	Digest   string       `json:"digest"`
	Patch    string       `json:"patch"`
}

type ApplyWorktreeHandoffRequest struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}
