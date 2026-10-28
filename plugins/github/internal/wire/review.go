package wire

import (
	"time"
)

type PullListInput struct {
	ListInput
	Base *string `json:"base,omitzero" jsonschema:"minLength=1,maxLength=1024"`
	Head *string `json:"head,omitzero" jsonschema:"minLength=1,maxLength=1024"`
}

type ThreadInput struct {
	IssueInput
	Page  int64 `json:"page" jsonschema:"minimum=1"`
	Limit int   `json:"limit" jsonschema:"minimum=1,maximum=100"`
}

type CommitInput struct {
	Repository string `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	Ref        string `json:"ref" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Use the exact pull request head SHA to inspect checks for that revision."`
	Page       int64  `json:"page" jsonschema:"minimum=1"`
	Limit      int    `json:"limit" jsonschema:"minimum=1,maximum=100"`
}

type CheckInput struct {
	Repository string `json:"repository" jsonschema:"minLength=3,maxLength=256"`
	CheckRunID int64  `json:"checkRunId" jsonschema:"minimum=1"`
	Page       int64  `json:"page" jsonschema:"minimum=1"`
	Limit      int    `json:"limit" jsonschema:"minimum=1,maximum=100"`
}

type Branch struct {
	Label      string  `json:"label"`
	Ref        string  `json:"ref"`
	SHA        string  `json:"sha"`
	Repository *string `json:"repository,omitzero"`
}

type PullRequest struct {
	Repository          string     `json:"repository"`
	Number              int64      `json:"number"`
	Title               string     `json:"title"`
	Body                string     `json:"body"`
	State               State      `json:"state"`
	URL                 string     `json:"url"`
	Author              string     `json:"author"`
	Draft               bool       `json:"draft"`
	MaintainerCanModify *bool      `json:"maintainerCanModify,omitzero"`
	Head                Branch     `json:"head"`
	Base                Branch     `json:"base"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	MergedAt            *time.Time `json:"mergedAt,omitzero"`
	Mergeable           *bool      `json:"mergeable,omitzero"`
	MergeableState      *string    `json:"mergeableState,omitzero"`
	Additions           *int64     `json:"additions,omitzero"`
	Deletions           *int64     `json:"deletions,omitzero"`
	ChangedFiles        *int64     `json:"changedFiles,omitzero"`
}

type PullPage struct {
	PullRequests []PullRequest `json:"pullRequests"`
	NextPage     *int64        `json:"nextPage,omitzero"`
}

type ChangedFile struct {
	Path         string  `json:"path"`
	PreviousPath *string `json:"previousPath,omitzero"`
	SHA          string  `json:"sha"`
	Status       string  `json:"status"`
	Additions    int64   `json:"additions"`
	Deletions    int64   `json:"deletions"`
	Changes      int64   `json:"changes"`
	Patch        *string `json:"patch,omitzero"`
}

type FilePage struct {
	Files        []ChangedFile `json:"files"`
	NextPage     *int64        `json:"nextPage,omitzero"`
	LimitReached bool          `json:"limitReached"`
}

type Comment struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CommentPage struct {
	Comments []Comment `json:"comments"`
	NextPage *int64    `json:"nextPage,omitzero"`
}

type Review struct {
	ID          int64      `json:"id"`
	URL         string     `json:"url"`
	Author      string     `json:"author"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	CommitID    *string    `json:"commitId,omitzero"`
	SubmittedAt *time.Time `json:"submittedAt,omitzero"`
}

type ReviewPage struct {
	Reviews  []Review `json:"reviews"`
	NextPage *int64   `json:"nextPage,omitzero"`
}

type ReviewComment struct {
	Comment
	ReviewID         int64   `json:"reviewId"`
	Path             string  `json:"path"`
	DiffHunk         string  `json:"diffHunk"`
	CommitID         string  `json:"commitId"`
	OriginalCommitID string  `json:"originalCommitId"`
	Line             *int64  `json:"line,omitzero"`
	OriginalLine     *int64  `json:"originalLine,omitzero"`
	Side             string  `json:"side"`
	StartLine        *int64  `json:"startLine,omitzero"`
	StartSide        *string `json:"startSide,omitzero"`
	InReplyTo        *int64  `json:"inReplyTo,omitzero"`
}

type ReviewCommentPage struct {
	Comments []ReviewComment `json:"comments"`
	NextPage *int64          `json:"nextPage,omitzero"`
}

type CheckRun struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	HeadSHA          string     `json:"headSha"`
	Status           string     `json:"status"`
	Conclusion       *string    `json:"conclusion,omitzero"`
	URL              string     `json:"url"`
	StartedAt        *time.Time `json:"startedAt,omitzero"`
	CompletedAt      *time.Time `json:"completedAt,omitzero"`
	Title            string     `json:"title"`
	Summary          string     `json:"summary"`
	Text             string     `json:"text"`
	AnnotationsCount int64      `json:"annotationsCount"`
}

type CheckPage struct {
	CheckRuns []CheckRun `json:"checkRuns"`
	NextPage  *int64     `json:"nextPage,omitzero"`
}

type Annotation struct {
	Path        string  `json:"path"`
	StartLine   int64   `json:"startLine"`
	EndLine     int64   `json:"endLine"`
	StartColumn *int64  `json:"startColumn,omitzero"`
	EndColumn   *int64  `json:"endColumn,omitzero"`
	Level       string  `json:"level"`
	Message     string  `json:"message"`
	Title       *string `json:"title,omitzero"`
	RawDetails  *string `json:"rawDetails,omitzero"`
}

type AnnotationPage struct {
	Annotations []Annotation `json:"annotations"`
	NextPage    *int64       `json:"nextPage,omitzero"`
}

type CommitStatus struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"`
	Context     string    `json:"context"`
	Description *string   `json:"description,omitzero"`
	URL         *string   `json:"url,omitzero"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type StatusPage struct {
	Statuses []CommitStatus `json:"statuses"`
	NextPage *int64         `json:"nextPage,omitzero"`
}
