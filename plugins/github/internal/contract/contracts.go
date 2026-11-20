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
	capability, err := plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability:          capability,
		List:                pluginmcp.Implement[wire.ListInput, wire.IssuePage](methods, published.ListIssuesV1Method),
		Issue:               pluginmcp.Implement[wire.IssueInput, wire.Issue](methods, published.GetIssueV1Method),
		File:                pluginmcp.Implement[wire.FileInput, wire.File](methods, published.ReadFileV1Method),
		Pulls:               pluginmcp.Implement[wire.PullListInput, wire.PullPage](methods, published.ListPullRequestsV1Method),
		Pull:                pluginmcp.Implement[wire.IssueInput, wire.PullRequest](methods, published.GetPullRequestV1Method),
		PullFiles:           pluginmcp.Implement[wire.ThreadInput, wire.FilePage](methods, published.ListPullRequestFilesV1Method),
		Comments:            pluginmcp.Implement[wire.ThreadInput, wire.CommentPage](methods, published.ListIssueCommentsV1Method),
		Reviews:             pluginmcp.Implement[wire.ThreadInput, wire.ReviewPage](methods, published.ListPullRequestReviewsV1Method),
		ReviewComments:      pluginmcp.Implement[wire.ThreadInput, wire.ReviewCommentPage](methods, published.ListPullRequestCommentsV1Method),
		Checks:              pluginmcp.Implement[wire.CommitInput, wire.CheckPage](methods, published.ListCheckRunsV1Method),
		Annotations:         pluginmcp.Implement[wire.CheckInput, wire.AnnotationPage](methods, published.ListCheckAnnotationsV1Method),
		Statuses:            pluginmcp.Implement[wire.CommitInput, wire.StatusPage](methods, published.ListCommitStatusesV1Method),
		CreateIssue:         pluginmcp.Implement[wire.CreateIssueInput, wire.Issue](methods, published.CreateIssueV1Method),
		UpdateIssue:         pluginmcp.Implement[wire.UpdateIssueInput, wire.Issue](methods, published.UpdateIssueV1Method),
		CreateComment:       pluginmcp.Implement[wire.CreateCommentInput, wire.Comment](methods, published.CreateIssueCommentV1Method),
		UpdateComment:       pluginmcp.Implement[wire.UpdateCommentInput, wire.Comment](methods, published.UpdateIssueCommentV1Method),
		DeleteComment:       pluginmcp.Implement[wire.CommentInput, wire.DeletedComment](methods, published.DeleteIssueCommentV1Method),
		CreatePull:          pluginmcp.Implement[wire.CreatePullInput, wire.PullRequest](methods, published.CreatePullRequestV1Method),
		SubmitReview:        pluginmcp.Implement[wire.SubmitReviewInput, wire.Review](methods, published.SubmitPullRequestReviewV1Method),
		MergePull:           pluginmcp.Implement[wire.MergePullInput, wire.MergeResult](methods, published.MergePullRequestV1Method),
		CreateReviewComment: pluginmcp.Implement[wire.CreateReviewCommentInput, wire.ReviewComment](methods, published.CreatePullRequestCommentV1Method),
		ReplyReviewComment:  pluginmcp.Implement[wire.ReplyReviewCommentInput, wire.ReviewComment](methods, published.ReplyPullRequestCommentV1Method),
		UpdateReviewComment: pluginmcp.Implement[wire.UpdateCommentInput, wire.ReviewComment](methods, published.UpdatePullRequestCommentV1Method),
		DeleteReviewComment: pluginmcp.Implement[wire.CommentInput, wire.DeletedComment](methods, published.DeletePullRequestCommentV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
