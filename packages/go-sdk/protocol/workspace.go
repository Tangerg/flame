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
	Ref          WorkspaceRef          `json:"ref"`
	RepositoryRoot  string                `json:"repositoryRoot,omitempty"`
	Availability WorkspaceAvailability `json:"availability"`
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
