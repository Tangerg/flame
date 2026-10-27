package wire

import (
	"github.com/invopop/jsonschema"
)

const (
	MaxBodyRunes  = 65536
	MaxTitleRunes = 256
	MaxWriteBytes = 1 << 20
	CommitPattern = `^[0-9a-fA-F]{40}$`
)

type CreateIssueInput struct {
	Repository string   `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	Title      string   `json:"title" jsonschema:"minLength=1"`
	Body       *string  `json:"body,omitzero"`
	Labels     []string `json:"labels,omitzero"`
	Assignees  []string `json:"assignees,omitzero"`
}

type StateReason string

const (
	Completed  StateReason = "completed"
	NotPlanned StateReason = "not_planned"
	Duplicate  StateReason = "duplicate"
	Reopened   StateReason = "reopened"
)

func (StateReason) JSONSchemaEnum() []string {
	return []string{string(Completed), string(NotPlanned), string(Duplicate), string(Reopened)}
}

type UpdateIssueInput struct {
	IssueInput
	Title            *string      `json:"title,omitzero" jsonschema:"minLength=1"`
	Body             *string      `json:"body,omitzero"`
	State            *State       `json:"state,omitzero"`
	StateReason      *StateReason `json:"stateReason,omitzero"`
	DuplicateIssueID *int64       `json:"duplicateIssueId,omitzero" jsonschema:"minimum=1" jsonschema_description:"Global id returned by get_issue for the canonical duplicate, not its issue number."`
	Labels           *[]string    `json:"labels,omitzero" jsonschema_description:"Replaces the label set; [] clears it. Omit to preserve it."`
	Assignees        *[]string    `json:"assignees,omitzero" jsonschema_description:"Replaces the assignee set; [] clears it. Omit to preserve it."`
}

type CreateCommentInput struct {
	IssueInput
	Body string `json:"body" jsonschema:"minLength=1"`
}

type CommentInput struct {
	Repository string `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	CommentID  int64  `json:"commentId" jsonschema:"minimum=1"`
}

type UpdateCommentInput struct {
	CommentInput
	Body string `json:"body" jsonschema:"minLength=1"`
}

type DeletedComment struct {
	ID      int64 `json:"id" jsonschema:"minimum=1"`
	Deleted bool  `json:"deleted" `
}

type CreatePullInput struct {
	Repository          string  `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	Title               string  `json:"title" jsonschema:"minLength=1"`
	Head                string  `json:"head" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Existing source branch, qualified as owner:branch for a fork."`
	Base                string  `json:"base" jsonschema:"minLength=1,maxLength=1024"`
	HeadRepository      *string `json:"headRepository,omitzero" jsonschema:"minLength=1"`
	Body                *string `json:"body,omitzero"`
	Draft               *bool   `json:"draft,omitzero"`
	MaintainerCanModify *bool   `json:"maintainerCanModify,omitzero"`
}

type ReviewEvent string

const (
	Approve        ReviewEvent = "APPROVE"
	RequestChanges ReviewEvent = "REQUEST_CHANGES"
	CommentReview  ReviewEvent = "COMMENT"
)

func (ReviewEvent) JSONSchemaEnum() []string {
	return []string{string(Approve), string(RequestChanges), string(CommentReview)}
}

type DiffSide string

const (
	Left  DiffSide = "LEFT"
	Right DiffSide = "RIGHT"
)

func (DiffSide) JSONSchemaEnum() []string { return []string{string(Left), string(Right)} }

type InlineComment struct {
	Path      string    `json:"path" jsonschema:"minLength=1,maxLength=4096"`
	Body      string    `json:"body" jsonschema:"minLength=1"`
	Line      int64     `json:"line" jsonschema:"minimum=1"`
	Side      DiffSide  `json:"side"`
	StartLine *int64    `json:"startLine,omitzero" jsonschema:"minimum=1"`
	StartSide *DiffSide `json:"startSide,omitzero"`
}

type SubmitReviewInput struct {
	IssueInput
	CommitID string          `json:"commitId" jsonschema:"minLength=1" jsonschema_description:"Exact inspected commit SHA to review; never defaults to a newer head."`
	Event    ReviewEvent     `json:"event"`
	Body     *string         `json:"body,omitzero"`
	Comments []InlineComment `json:"comments,omitzero"`
}

type CreateReviewCommentInput struct {
	IssueInput
	CommitID string `json:"commitId" jsonschema:"minLength=1"`
	InlineComment
}

type ReplyReviewCommentInput struct {
	IssueInput
	CommentID int64  `json:"commentId" jsonschema:"minimum=1"`
	Body      string `json:"body" jsonschema:"minLength=1"`
}

type MergeMethod string

const (
	Merge  MergeMethod = "merge"
	Squash MergeMethod = "squash"
	Rebase MergeMethod = "rebase"
)

func (MergeMethod) JSONSchemaEnum() []string {
	return []string{string(Merge), string(Squash), string(Rebase)}
}

type MergePullInput struct {
	IssueInput
	HeadSHA       string      `json:"headSha" jsonschema:"minLength=1" jsonschema_description:"Exact inspected head SHA. GitHub atomically refuses a different current head."`
	MergeMethod   MergeMethod `json:"mergeMethod"`
	CommitTitle   *string     `json:"commitTitle,omitzero"`
	CommitMessage *string     `json:"commitMessage,omitzero"`
}

type MergeResult struct {
	SHA     string `json:"sha" jsonschema:"minLength=1"`
	Merged  bool   `json:"merged" `
	Message string `json:"message"`
}

func boundString(schema *jsonschema.Schema, field string, maximum int) {
	if property, ok := schema.Properties.Get(field); ok {
		property.MaxLength = new(uint64(maximum))
	}
}

func boundCommit(schema *jsonschema.Schema, field string) {
	if property, ok := schema.Properties.Get(field); ok {
		property.Pattern = CommitPattern
	}
}

func (CreateIssueInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "title", MaxTitleRunes)
	boundString(schema, "body", MaxBodyRunes)
}

func (UpdateIssueInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "title", MaxTitleRunes)
	boundString(schema, "body", MaxBodyRunes)
}

func (CreateCommentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
}

func (UpdateCommentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
}

func (CreatePullInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "title", MaxTitleRunes)
	boundString(schema, "body", MaxBodyRunes)
}

func (InlineComment) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
}

func (SubmitReviewInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
	boundCommit(schema, "commitId")
}

func (CreateReviewCommentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
	boundCommit(schema, "commitId")
}

func (ReplyReviewCommentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "body", MaxBodyRunes)
}

func (MergePullInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	boundString(schema, "commitTitle", MaxTitleRunes)
	boundString(schema, "commitMessage", MaxBodyRunes)
	boundCommit(schema, "headSha")
}

func (DeletedComment) JSONSchemaExtend(schema *jsonschema.Schema) {
	if property, ok := schema.Properties.Get("deleted"); ok {
		property.Const = true
	}
}

func (MergeResult) JSONSchemaExtend(schema *jsonschema.Schema) {
	if property, ok := schema.Properties.Get("merged"); ok {
		property.Const = true
	}
}
