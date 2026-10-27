package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/github/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	published "github.com/Tangerg/flame/plugins/github/contracts"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability          *portable.Capability
	List                *qimumcp.Method[wire.ListInput, wire.IssuePage]
	Issue               *qimumcp.Method[wire.IssueInput, wire.Issue]
	File                *qimumcp.Method[wire.FileInput, wire.File]
	Pulls               *qimumcp.Method[wire.PullListInput, wire.PullPage]
	Pull                *qimumcp.Method[wire.IssueInput, wire.PullRequest]
	PullFiles           *qimumcp.Method[wire.ThreadInput, wire.FilePage]
	Comments            *qimumcp.Method[wire.ThreadInput, wire.CommentPage]
	Reviews             *qimumcp.Method[wire.ThreadInput, wire.ReviewPage]
	ReviewComments      *qimumcp.Method[wire.ThreadInput, wire.ReviewCommentPage]
	Checks              *qimumcp.Method[wire.CommitInput, wire.CheckPage]
	Annotations         *qimumcp.Method[wire.CheckInput, wire.AnnotationPage]
	Statuses            *qimumcp.Method[wire.CommitInput, wire.StatusPage]
	CreateIssue         *qimumcp.Method[wire.CreateIssueInput, wire.Issue]
	UpdateIssue         *qimumcp.Method[wire.UpdateIssueInput, wire.Issue]
	CreateComment       *qimumcp.Method[wire.CreateCommentInput, wire.Comment]
	UpdateComment       *qimumcp.Method[wire.UpdateCommentInput, wire.Comment]
	DeleteComment       *qimumcp.Method[wire.CommentInput, wire.DeletedComment]
	CreatePull          *qimumcp.Method[wire.CreatePullInput, wire.PullRequest]
	SubmitReview        *qimumcp.Method[wire.SubmitReviewInput, wire.Review]
	MergePull           *qimumcp.Method[wire.MergePullInput, wire.MergeResult]
	CreateReviewComment *qimumcp.Method[wire.CreateReviewCommentInput, wire.ReviewComment]
	ReplyReviewComment  *qimumcp.Method[wire.ReplyReviewCommentInput, wire.ReviewComment]
	UpdateReviewComment *qimumcp.Method[wire.UpdateCommentInput, wire.ReviewComment]
	DeleteReviewComment *qimumcp.Method[wire.CommentInput, wire.DeletedComment]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = qimumcp.ImplementMethod[wire.ListInput, wire.IssuePage](c.Capability, published.ListIssuesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Issue, err = qimumcp.ImplementMethod[wire.IssueInput, wire.Issue](c.Capability, published.GetIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.File, err = qimumcp.ImplementMethod[wire.FileInput, wire.File](c.Capability, published.ReadFileV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Pulls, err = qimumcp.ImplementMethod[wire.PullListInput, wire.PullPage](c.Capability, published.ListPullRequestsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Pull, err = qimumcp.ImplementMethod[wire.IssueInput, wire.PullRequest](c.Capability, published.GetPullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.PullFiles, err = qimumcp.ImplementMethod[wire.ThreadInput, wire.FilePage](c.Capability, published.ListPullRequestFilesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Comments, err = qimumcp.ImplementMethod[wire.ThreadInput, wire.CommentPage](c.Capability, published.ListIssueCommentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Reviews, err = qimumcp.ImplementMethod[wire.ThreadInput, wire.ReviewPage](c.Capability, published.ListPullRequestReviewsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.ReviewComments, err = qimumcp.ImplementMethod[wire.ThreadInput, wire.ReviewCommentPage](c.Capability, published.ListPullRequestCommentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Checks, err = qimumcp.ImplementMethod[wire.CommitInput, wire.CheckPage](c.Capability, published.ListCheckRunsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Annotations, err = qimumcp.ImplementMethod[wire.CheckInput, wire.AnnotationPage](c.Capability, published.ListCheckAnnotationsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Statuses, err = qimumcp.ImplementMethod[wire.CommitInput, wire.StatusPage](c.Capability, published.ListCommitStatusesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateIssue, err = qimumcp.ImplementMethod[wire.CreateIssueInput, wire.Issue](c.Capability, published.CreateIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateIssue, err = qimumcp.ImplementMethod[wire.UpdateIssueInput, wire.Issue](c.Capability, published.UpdateIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateComment, err = qimumcp.ImplementMethod[wire.CreateCommentInput, wire.Comment](c.Capability, published.CreateIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateComment, err = qimumcp.ImplementMethod[wire.UpdateCommentInput, wire.Comment](c.Capability, published.UpdateIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.DeleteComment, err = qimumcp.ImplementMethod[wire.CommentInput, wire.DeletedComment](c.Capability, published.DeleteIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreatePull, err = qimumcp.ImplementMethod[wire.CreatePullInput, wire.PullRequest](c.Capability, published.CreatePullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.SubmitReview, err = qimumcp.ImplementMethod[wire.SubmitReviewInput, wire.Review](c.Capability, published.SubmitPullRequestReviewV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.MergePull, err = qimumcp.ImplementMethod[wire.MergePullInput, wire.MergeResult](c.Capability, published.MergePullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateReviewComment, err = qimumcp.ImplementMethod[wire.CreateReviewCommentInput, wire.ReviewComment](c.Capability, published.CreatePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.ReplyReviewComment, err = qimumcp.ImplementMethod[wire.ReplyReviewCommentInput, wire.ReviewComment](c.Capability, published.ReplyPullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateReviewComment, err = qimumcp.ImplementMethod[wire.UpdateCommentInput, wire.ReviewComment](c.Capability, published.UpdatePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.DeleteReviewComment, err = qimumcp.ImplementMethod[wire.CommentInput, wire.DeletedComment](c.Capability, published.DeletePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
