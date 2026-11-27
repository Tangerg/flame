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
	Archived *bool `json:"archived,omitzero"`
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

// BackgroundSource describes the immutable event captured by an installed trigger.
type BackgroundSource struct {
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Version   string `json:"version"`
	Title     string `json:"title"`
}

type ProjectBackgroundPolicy struct {
	Revision     int64    `json:"revision"`
	Paused       bool     `json:"paused"`
	MaxActive    int      `json:"maxActive"`
	MaxDailyRuns int      `json:"maxDailyRuns"`
	PullRequests []string `json:"pullRequests"`
}

type SetProjectBackgroundPolicyRequest struct {
	ProjectID string                  `json:"projectId"`
	Policy    ProjectBackgroundPolicy `json:"policy"`
}

type ProjectBackgroundRun struct {
	Source     BackgroundSource `json:"source"`
	SessionID  string           `json:"sessionId"`
	RunID      string           `json:"runId"`
	Status     string           `json:"status"`
	Outcome    string           `json:"outcome,omitempty"`
	AdmittedAt time.Time        `json:"admittedAt"`
}

type ProjectBackgroundActivity struct {
	Policy   ProjectBackgroundPolicy `json:"policy"`
	Active   int                     `json:"active"`
	Admitted int                     `json:"admitted"`
	Recent   []ProjectBackgroundRun  `json:"recent"`
}
