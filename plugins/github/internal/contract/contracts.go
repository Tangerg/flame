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

type Contracts struct {
	Capability          *extension.Capability
	List                *pluginsdk.Method[ListInput, IssuePage]
	Issue               *pluginsdk.Method[IssueInput, Issue]
	File                *pluginsdk.Method[FileInput, File]
	Pulls               *pluginsdk.Method[PullListInput, PullPage]
	Pull                *pluginsdk.Method[IssueInput, PullRequest]
	PullFiles           *pluginsdk.Method[ThreadInput, FilePage]
	Comments            *pluginsdk.Method[ThreadInput, CommentPage]
	Reviews             *pluginsdk.Method[ThreadInput, ReviewPage]
	ReviewComments      *pluginsdk.Method[ThreadInput, ReviewCommentPage]
	Checks              *pluginsdk.Method[CommitInput, CheckPage]
	Annotations         *pluginsdk.Method[CheckInput, AnnotationPage]
	Statuses            *pluginsdk.Method[CommitInput, StatusPage]
	CreateIssue         *pluginsdk.Method[CreateIssueInput, Issue]
	UpdateIssue         *pluginsdk.Method[UpdateIssueInput, Issue]
	CreateComment       *pluginsdk.Method[CreateCommentInput, Comment]
	UpdateComment       *pluginsdk.Method[UpdateCommentInput, Comment]
	DeleteComment       *pluginsdk.Method[CommentInput, DeletedComment]
	CreatePull          *pluginsdk.Method[CreatePullInput, PullRequest]
	SubmitReview        *pluginsdk.Method[SubmitReviewInput, Review]
	MergePull           *pluginsdk.Method[MergePullInput, MergeResult]
	CreateReviewComment *pluginsdk.Method[CreateReviewCommentInput, ReviewComment]
	ReplyReviewComment  *pluginsdk.Method[ReplyReviewCommentInput, ReviewComment]
	UpdateReviewComment *pluginsdk.Method[UpdateCommentInput, ReviewComment]
	DeleteReviewComment *pluginsdk.Method[CommentInput, DeletedComment]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.List, err = readMethod[ListInput, IssuePage]("list-issues", "Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub.")
	if err != nil {
		return Contracts{}, err
	}
	c.Issue, err = readMethod[IssueInput, Issue]("get-issue", "Read one GitHub issue or pull request by its repository and issue number.")
	if err != nil {
		return Contracts{}, err
	}
	c.File, err = readMethod[FileInput, File]("read-file", "Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused.")
	if err != nil {
		return Contracts{}, err
	}
	c.Pulls, err = readMethod[PullListInput, PullPage]("list-pull-requests", "List one page of pull requests with head/base refs and SHAs, optionally filtered by base or owner:branch head.")
	if err != nil {
		return Contracts{}, err
	}
	c.Pull, err = readMethod[IssueInput, PullRequest]("get-pull-request", "Read a pull request's head/base SHAs, draft/merge state and change counts. A missing mergeable value means GitHub has not computed it. Use the head SHA for subsequent commit checks.")
	if err != nil {
		return Contracts{}, err
	}
	c.PullFiles, err = readMethod[ThreadInput, FilePage]("list-pull-request-files", "Read one live page of PR file changes and available patches. An absent patch is not an empty diff. GitHub exposes at most 3000 files; limitReached means the list may be incomplete. Read repository files at an exact PR head/base SHA when needed.")
	if err != nil {
		return Contracts{}, err
	}
	c.Comments, err = readMethod[ThreadInput, CommentPage]("list-issue-comments", "Read one page of discussion comments on an issue or pull request. Inline code review comments are returned by list_pull_request_comments.")
	if err != nil {
		return Contracts{}, err
	}
	c.Reviews, err = readMethod[ThreadInput, ReviewPage]("list-pull-request-reviews", "Read one page of PR reviews with their upstream state, commit and submitted time, including pending and dismissed reviews.")
	if err != nil {
		return Contracts{}, err
	}
	c.ReviewComments, err = readMethod[ThreadInput, ReviewCommentPage]("list-pull-request-comments", "Read one page of inline PR review comments with diff context, commit identity, reply parent and available line positions. Missing current positions can indicate outdated comments.")
	if err != nil {
		return Contracts{}, err
	}
	c.Checks, err = readMethod[CommitInput, CheckPage]("list-check-runs", "Read one page of latest check runs with upstream status, conclusion and diagnostic text. GitHub limits this endpoint to the most recent 1000 check suites. Use the exact PR head SHA, list_check_annotations for line diagnostics, and list_commit_statuses for external CI.")
	if err != nil {
		return Contracts{}, err
	}
	c.Annotations, err = readMethod[CheckInput, AnnotationPage]("list-check-annotations", "Read one page of line-level diagnostics for a check run ID returned by list_check_runs. Preserves paths, ranges, severity and original diagnostic text.")
	if err != nil {
		return Contracts{}, err
	}
	c.Statuses, err = readMethod[CommitInput, StatusPage]("list-commit-statuses", "Read one page of commit statuses, including external CI that is not represented by check runs. Use an exact PR head SHA when assessing a revision.")
	if err != nil {
		return Contracts{}, err
	}
	writes, err := c.writeMethods()
	if err != nil {
		return Contracts{}, err
	}
	methods := []extension.MethodDescriptor{c.List.Descriptor(), c.Issue.Descriptor(), c.File.Descriptor(), c.Pulls.Descriptor(), c.Pull.Descriptor(), c.PullFiles.Descriptor(), c.Comments.Descriptor(), c.Reviews.Descriptor(), c.ReviewComments.Descriptor(), c.Checks.Descriptor(), c.Annotations.Descriptor(), c.Statuses.Descriptor()}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.github.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, append(methods, writes...)...)
	return c, err
}

func readMethod[P, R any](name, description string) (*pluginsdk.Method[P, R], error) {
	return pluginsdk.NewMethod[P, R](pluginsdk.MethodConfig{Name: "flame.github/" + name + ".v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: description})
}
