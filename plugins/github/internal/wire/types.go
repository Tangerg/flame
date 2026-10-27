package wire

import (
	"time"
)

type State string

const (
	Open   State = "open"
	Closed State = "closed"
)

func (State) JSONSchemaEnum() []string { return []string{string(Open), string(Closed)} }

type Filter string

func (Filter) JSONSchemaEnum() []string { return []string{string(Open), string(Closed), "all"} }

type ListInput struct {
	Repository string `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	State      Filter `json:"state"`
	Page       int64  `json:"page" jsonschema:"minimum=1"`
	Limit      int    `json:"limit" jsonschema:"minimum=1,maximum=100"`
}

type IssueInput struct {
	Repository string `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	Number     int64  `json:"number" jsonschema:"minimum=1"`
}

type FileInput struct {
	Repository string  `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	Path       string  `json:"path" jsonschema:"minLength=1,maxLength=4096"`
	Ref        *string `json:"ref,omitzero" jsonschema:"minLength=1,maxLength=1024"`
}

type Issue struct {
	ID          int64     `json:"id" jsonschema:"minimum=1"`
	Repository  string    `json:"repository"`
	Number      int64     `json:"number" jsonschema:"minimum=1"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       State     `json:"state"`
	URL         string    `json:"url"`
	Author      string    `json:"author"`
	PullRequest bool      `json:"pullRequest"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Labels      []string  `json:"labels"`
	Assignees   []string  `json:"assignees"`
	StateReason *string   `json:"stateReason,omitzero"`
}

type IssuePage struct {
	Issues   []Issue `json:"issues"`
	NextPage *int64  `json:"nextPage,omitzero"`
}

type File struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	SHA        string `json:"sha"`
	URL        string `json:"url"`
	Text       string `json:"text"`
}
