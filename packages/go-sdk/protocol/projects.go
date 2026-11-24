package protocol

import "time"

type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Instructions string    `json:"instructions,omitempty"`
	Archived     bool      `json:"archived,omitzero"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Revision     uint64    `json:"revision"`
}

type ListProjectsRequest struct {
	PageQuery
	Archived bool `json:"archived,omitzero"`
}

type GetProjectRequest struct {
	ProjectID string `json:"projectId"`
}

type CreateProjectRequest struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions,omitempty"`
}

type UpdateProjectRequest struct {
	ProjectID        string  `json:"projectId"`
	ExpectedRevision uint64  `json:"expectedRevision"`
	Name             *string `json:"name,omitzero"`
	Instructions     *string `json:"instructions,omitzero"`
	Archived         *bool   `json:"archived,omitzero"`
}

//flame:enum
type SessionProjectChangeType string

const (
	SessionProjectSet   SessionProjectChangeType = "set"
	SessionProjectClear SessionProjectChangeType = "clear"
)

type SessionProjectChange struct {
	Type      SessionProjectChangeType `json:"type"`
	ProjectID string                   `json:"projectId,omitempty"`
}
