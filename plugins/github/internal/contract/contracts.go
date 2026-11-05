package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/github/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	published "github.com/Tangerg/flame/plugins/github/contracts"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

type Contracts struct {
	Capability          *plugincontract.Capability
	List                *pluginmcp.Method[wire.ListInput, wire.IssuePage]
	Issue               *pluginmcp.Method[wire.IssueInput, wire.Issue]
	File                *pluginmcp.Method[wire.FileInput, wire.File]
	Pulls               *pluginmcp.Method[wire.PullListInput, wire.PullPage]
	Pull                *pluginmcp.Method[wire.IssueInput, wire.PullRequest]
	PullFiles           *pluginmcp.Method[wire.ThreadInput, wire.FilePage]
	Comments            *pluginmcp.Method[wire.ThreadInput, wire.CommentPage]
	Reviews             *pluginmcp.Method[wire.ThreadInput, wire.ReviewPage]
	ReviewComments      *pluginmcp.Method[wire.ThreadInput, wire.ReviewCommentPage]
	Checks              *pluginmcp.Method[wire.CommitInput, wire.CheckPage]
	Annotations         *pluginmcp.Method[wire.CheckInput, wire.AnnotationPage]
	Statuses            *pluginmcp.Method[wire.CommitInput, wire.StatusPage]
	CreateIssue         *pluginmcp.Method[wire.CreateIssueInput, wire.Issue]
	UpdateIssue         *pluginmcp.Method[wire.UpdateIssueInput, wire.Issue]
	CreateComment       *pluginmcp.Method[wire.CreateCommentInput, wire.Comment]
	UpdateComment       *pluginmcp.Method[wire.UpdateCommentInput, wire.Comment]
	DeleteComment       *pluginmcp.Method[wire.CommentInput, wire.DeletedComment]
	CreatePull          *pluginmcp.Method[wire.CreatePullInput, wire.PullRequest]
	SubmitReview        *pluginmcp.Method[wire.SubmitReviewInput, wire.Review]
	MergePull           *pluginmcp.Method[wire.MergePullInput, wire.MergeResult]
	CreateReviewComment *pluginmcp.Method[wire.CreateReviewCommentInput, wire.ReviewComment]
	ReplyReviewComment  *pluginmcp.Method[wire.ReplyReviewCommentInput, wire.ReviewComment]
	UpdateReviewComment *pluginmcp.Method[wire.UpdateCommentInput, wire.ReviewComment]
	DeleteReviewComment *pluginmcp.Method[wire.CommentInput, wire.DeletedComment]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginmcp.ImplementMethod[wire.ListInput, wire.IssuePage](c.Capability, published.ListIssuesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Issue, err = pluginmcp.ImplementMethod[wire.IssueInput, wire.Issue](c.Capability, published.GetIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.File, err = pluginmcp.ImplementMethod[wire.FileInput, wire.File](c.Capability, published.ReadFileV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Pulls, err = pluginmcp.ImplementMethod[wire.PullListInput, wire.PullPage](c.Capability, published.ListPullRequestsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Pull, err = pluginmcp.ImplementMethod[wire.IssueInput, wire.PullRequest](c.Capability, published.GetPullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.PullFiles, err = pluginmcp.ImplementMethod[wire.ThreadInput, wire.FilePage](c.Capability, published.ListPullRequestFilesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Comments, err = pluginmcp.ImplementMethod[wire.ThreadInput, wire.CommentPage](c.Capability, published.ListIssueCommentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Reviews, err = pluginmcp.ImplementMethod[wire.ThreadInput, wire.ReviewPage](c.Capability, published.ListPullRequestReviewsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.ReviewComments, err = pluginmcp.ImplementMethod[wire.ThreadInput, wire.ReviewCommentPage](c.Capability, published.ListPullRequestCommentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Checks, err = pluginmcp.ImplementMethod[wire.CommitInput, wire.CheckPage](c.Capability, published.ListCheckRunsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Annotations, err = pluginmcp.ImplementMethod[wire.CheckInput, wire.AnnotationPage](c.Capability, published.ListCheckAnnotationsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Statuses, err = pluginmcp.ImplementMethod[wire.CommitInput, wire.StatusPage](c.Capability, published.ListCommitStatusesV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateIssue, err = pluginmcp.ImplementMethod[wire.CreateIssueInput, wire.Issue](c.Capability, published.CreateIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateIssue, err = pluginmcp.ImplementMethod[wire.UpdateIssueInput, wire.Issue](c.Capability, published.UpdateIssueV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateComment, err = pluginmcp.ImplementMethod[wire.CreateCommentInput, wire.Comment](c.Capability, published.CreateIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateComment, err = pluginmcp.ImplementMethod[wire.UpdateCommentInput, wire.Comment](c.Capability, published.UpdateIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.DeleteComment, err = pluginmcp.ImplementMethod[wire.CommentInput, wire.DeletedComment](c.Capability, published.DeleteIssueCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreatePull, err = pluginmcp.ImplementMethod[wire.CreatePullInput, wire.PullRequest](c.Capability, published.CreatePullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.SubmitReview, err = pluginmcp.ImplementMethod[wire.SubmitReviewInput, wire.Review](c.Capability, published.SubmitPullRequestReviewV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.MergePull, err = pluginmcp.ImplementMethod[wire.MergePullInput, wire.MergeResult](c.Capability, published.MergePullRequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.CreateReviewComment, err = pluginmcp.ImplementMethod[wire.CreateReviewCommentInput, wire.ReviewComment](c.Capability, published.CreatePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.ReplyReviewComment, err = pluginmcp.ImplementMethod[wire.ReplyReviewCommentInput, wire.ReviewComment](c.Capability, published.ReplyPullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.UpdateReviewComment, err = pluginmcp.ImplementMethod[wire.UpdateCommentInput, wire.ReviewComment](c.Capability, published.UpdatePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.DeleteReviewComment, err = pluginmcp.ImplementMethod[wire.CommentInput, wire.DeletedComment](c.Capability, published.DeletePullRequestCommentV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
