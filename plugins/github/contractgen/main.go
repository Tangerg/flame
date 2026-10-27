package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*portable.Capability, error) {
	listMethod, err := readMethod[wire.ListInput, wire.IssuePage]("list-issues", "Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub.")
	if err != nil {
		return nil, err
	}
	issueMethod, err := readMethod[wire.IssueInput, wire.Issue]("get-issue", "Read one GitHub issue or pull request by its repository and issue number.")
	if err != nil {
		return nil, err
	}
	fileMethod, err := readMethod[wire.FileInput, wire.File]("read-file", "Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused.")
	if err != nil {
		return nil, err
	}
	pullsMethod, err := readMethod[wire.PullListInput, wire.PullPage]("list-pull-requests", "List one page of pull requests with head/base refs and SHAs, optionally filtered by base or owner:branch head.")
	if err != nil {
		return nil, err
	}
	pullMethod, err := readMethod[wire.IssueInput, wire.PullRequest]("get-pull-request", "Read a pull request's head/base SHAs, draft/merge state and change counts. A missing mergeable value means GitHub has not computed it. Use the head SHA for subsequent commit checks.")
	if err != nil {
		return nil, err
	}
	pullFilesMethod, err := readMethod[wire.ThreadInput, wire.FilePage]("list-pull-request-files", "Read one live page of PR file changes and available patches. An absent patch is not an empty diff. GitHub exposes at most 3000 files; limitReached means the list may be incomplete. Read repository files at an exact PR head/base SHA when needed.")
	if err != nil {
		return nil, err
	}
	commentsMethod, err := readMethod[wire.ThreadInput, wire.CommentPage]("list-issue-comments", "Read one page of discussion comments on an issue or pull request. Inline code review comments are returned by list_pull_request_comments.")
	if err != nil {
		return nil, err
	}
	reviewsMethod, err := readMethod[wire.ThreadInput, wire.ReviewPage]("list-pull-request-reviews", "Read one page of PR reviews with their upstream state, commit and submitted time, including pending and dismissed reviews.")
	if err != nil {
		return nil, err
	}
	reviewCommentsMethod, err := readMethod[wire.ThreadInput, wire.ReviewCommentPage]("list-pull-request-comments", "Read one page of inline PR review comments with diff context, commit identity, reply parent and available line positions. Missing current positions can indicate outdated comments.")
	if err != nil {
		return nil, err
	}
	checksMethod, err := readMethod[wire.CommitInput, wire.CheckPage]("list-check-runs", "Read one page of latest check runs with upstream status, conclusion and diagnostic text. GitHub limits this endpoint to the most recent 1000 check suites. Use the exact PR head SHA, list_check_annotations for line diagnostics, and list_commit_statuses for external CI.")
	if err != nil {
		return nil, err
	}
	annotationsMethod, err := readMethod[wire.CheckInput, wire.AnnotationPage]("list-check-annotations", "Read one page of line-level diagnostics for a check run ID returned by list_check_runs. Preserves paths, ranges, severity and original diagnostic text.")
	if err != nil {
		return nil, err
	}
	statusesMethod, err := readMethod[wire.CommitInput, wire.StatusPage]("list-commit-statuses", "Read one page of commit statuses, including external CI that is not represented by check runs. Use an exact PR head SHA when assessing a revision.")
	if err != nil {
		return nil, err
	}
	createIssueMethod, err := writeMethod[wire.CreateIssueInput, wire.Issue]("create-issue", "Create an issue with title, optional body, labels and assignees. Returns GitHub's actual values, including metadata GitHub may ignore without sufficient permissions.")
	if err != nil {
		return nil, err
	}
	updateIssueMethod, err := writeMethod[wire.UpdateIssueInput, wire.Issue]("update-issue", "Patch only supplied issue fields. Empty body clears it; empty labels/assignees clear their sets. Omitted fields are preserved. GitHub provides no atomic revision condition for these edits.")
	if err != nil {
		return nil, err
	}
	createCommentMethod, err := writeMethod[wire.CreateCommentInput, wire.Comment]("create-issue-comment", "Create a discussion comment on an issue or PR; code-line comments use create_pull_request_comment.")
	if err != nil {
		return nil, err
	}
	updateCommentMethod, err := writeMethod[wire.UpdateCommentInput, wire.Comment]("update-issue-comment", "Replace one issue/PR discussion comment's body by its inspected comment ID.")
	if err != nil {
		return nil, err
	}
	deleteCommentMethod, err := writeMethod[wire.CommentInput, wire.DeletedComment]("delete-issue-comment", "Delete one issue/PR discussion comment by its inspected ID. This does not delete an inline code-review comment.")
	if err != nil {
		return nil, err
	}
	createPullMethod, err := writeMethod[wire.CreatePullInput, wire.PullRequest]("create-pull-request", "Create a PR from existing head/base branches, optionally as a draft. This does not create commits or push a branch.")
	if err != nil {
		return nil, err
	}
	submitReviewMethod, err := writeMethod[wire.SubmitReviewInput, wire.Review]("submit-pull-request-review", "Create and submit an APPROVE, REQUEST_CHANGES or COMMENT review against the explicit inspected commit, optionally with inline comments. Body is required for COMMENT and REQUEST_CHANGES. Does not leave a pending review.")
	if err != nil {
		return nil, err
	}
	mergePullMethod, err := writeMethod[wire.MergePullInput, wire.MergeResult]("merge-pull-request", "Directly merge a PR with the explicit merge method and inspected head SHA. GitHub enforces protection and permissions. Merge queues and stacked PRs require GitHub's separate asynchronous API and are not supported by this operation.")
	if err != nil {
		return nil, err
	}
	createReviewCommentMethod, err := writeMethod[wire.CreateReviewCommentInput, wire.ReviewComment]("create-pull-request-comment", "Create an inline PR review comment at the inspected commit/path/line/side; optional startLine/startSide defines a range.")
	if err != nil {
		return nil, err
	}
	replyReviewCommentMethod, err := writeMethod[wire.ReplyReviewCommentInput, wire.ReviewComment]("reply-pull-request-comment", "Reply to an existing inline PR review comment by its inspected comment ID, preserving its upstream thread.")
	if err != nil {
		return nil, err
	}
	updateReviewCommentMethod, err := writeMethod[wire.UpdateCommentInput, wire.ReviewComment]("update-pull-request-comment", "Replace an inline PR review comment's body by its inspected comment ID.")
	if err != nil {
		return nil, err
	}
	deleteReviewCommentMethod, err := writeMethod[wire.CommentInput, wire.DeletedComment]("delete-pull-request-comment", "Delete an inline PR review comment by its inspected comment ID.")
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.github.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, listMethod.Descriptor(), issueMethod.Descriptor(), fileMethod.Descriptor(), pullsMethod.Descriptor(), pullMethod.Descriptor(), pullFilesMethod.Descriptor(), commentsMethod.Descriptor(), reviewsMethod.Descriptor(), reviewCommentsMethod.Descriptor(), checksMethod.Descriptor(), annotationsMethod.Descriptor(), statusesMethod.Descriptor(), createIssueMethod.Descriptor(), updateIssueMethod.Descriptor(), createCommentMethod.Descriptor(), updateCommentMethod.Descriptor(), deleteCommentMethod.Descriptor(), createPullMethod.Descriptor(), submitReviewMethod.Descriptor(), mergePullMethod.Descriptor(), createReviewCommentMethod.Descriptor(), replyReviewCommentMethod.Descriptor(), updateReviewCommentMethod.Descriptor(), deleteReviewCommentMethod.Descriptor())
}

func readMethod[P, R any](name, description string) (*qimumcp.Method[P, R], error) {
	return qimumcp.NewMethod[P, R](qimumcp.MethodConfig{Name: "flame.github/" + name + ".v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: description})
}

func writeMethod[P, R any](name, description string) (*qimumcp.Method[P, R], error) {
	return qimumcp.NewMethod[P, R](qimumcp.MethodConfig{Name: "flame.github/" + name + ".v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: description + " Requires the user's intent and a configured token. Recovery reads the original receipt; an unknown outcome never permits resubmission."})
}
