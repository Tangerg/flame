package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/github/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
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
	Repository  string    `json:"repository"`
	Number      int64     `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       State     `json:"state"`
	URL         string    `json:"url"`
	Author      string    `json:"author"`
	PullRequest bool      `json:"pullRequest"`
	UpdatedAt   time.Time `json:"updatedAt"`
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

type Contracts struct {
	Capability *extension.Capability
	List       *pluginsdk.Method[ListInput, IssuePage]
	Issue      *pluginsdk.Method[IssueInput, Issue]
	File       *pluginsdk.Method[FileInput, File]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.List, err = pluginsdk.NewMethod[ListInput, IssuePage](pluginsdk.MethodConfig{Name: "flame.github/list-issues.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub."})
	if err != nil {
		return Contracts{}, err
	}
	c.Issue, err = pluginsdk.NewMethod[IssueInput, Issue](pluginsdk.MethodConfig{Name: "flame.github/get-issue.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Read one GitHub issue or pull request by its repository and issue number."})
	if err != nil {
		return Contracts{}, err
	}
	c.File, err = pluginsdk.NewMethod[FileInput, File](pluginsdk.MethodConfig{Name: "flame.github/read-file.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.github.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.List.Descriptor(), c.Issue.Descriptor(), c.File.Descriptor())
	return c, err
}
