package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/github/internal/wire"

	"github.com/Tangerg/frame"
)

func main() { plugincontract.Publish(define) }

func define() (*plugincontract.Capability, error) {
	var declared declarations
	declared.add(query[wire.ListInput, wire.IssuePage]("list-issues", "Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub."))
	declared.add(query[wire.IssueInput, wire.Issue]("get-issue", "Read one GitHub issue or pull request by its repository and issue number."))
	declared.add(query[wire.FileInput, wire.File]("read-file", "Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused."))
	declared.add(query[wire.PullListInput, wire.PullPage]("list-pull-requests", "List one page of pull requests with head/base refs and SHAs, optionally filtered by base or owner:branch head."))
	declared.add(query[wire.IssueInput, wire.PullRequest]("get-pull-request", "Read a pull request's head/base SHAs, draft/merge state and change counts. A missing mergeable value means GitHub has not computed it. Use the head SHA for subsequent commit checks."))
	declared.add(query[wire.ThreadInput, wire.FilePage]("list-pull-request-files", "Read one live page of PR file changes and available patches. An absent patch is not an empty diff. GitHub exposes at most 3000 files; limitReached means the list may be incomplete. Read repository files at an exact PR head/base SHA when needed."))
	declared.add(query[wire.ThreadInput, wire.CommentPage]("list-issue-comments", "Read one page of discussion comments on an issue or pull request. Inline code review comments are returned by list_pull_request_comments."))
	declared.add(query[wire.ThreadInput, wire.ReviewPage]("list-pull-request-reviews", "Read one page of PR reviews with their upstream state, commit and submitted time, including pending and dismissed reviews."))
	declared.add(query[wire.ThreadInput, wire.ReviewCommentPage]("list-pull-request-comments", "Read one page of inline PR review comments with diff context, commit identity, reply parent and available line positions. Missing current positions can indicate outdated comments."))
	declared.add(query[wire.CommitInput, wire.CheckPage]("list-check-runs", "Read one page of latest check runs with upstream status, conclusion and diagnostic text. GitHub limits this endpoint to the most recent 1000 check suites. Use the exact PR head SHA, list_check_annotations for line diagnostics, and list_commit_statuses for external CI."))
	declared.add(query[wire.CheckInput, wire.AnnotationPage]("list-check-annotations", "Read one page of line-level diagnostics for a check run ID returned by list_check_runs. Preserves paths, ranges, severity and original diagnostic text."))
	declared.add(query[wire.CommitInput, wire.StatusPage]("list-commit-statuses", "Read one page of commit statuses, including external CI that is not represented by check runs. Use an exact PR head SHA when assessing a revision."))
	declared.add(command[wire.CreateIssueInput, wire.Issue]("create-issue", "Create an issue with title, optional body, labels and assignees. Returns GitHub's actual values, including metadata GitHub may ignore without sufficient permissions."))
	declared.add(command[wire.UpdateIssueInput, wire.Issue]("update-issue", "Patch only supplied issue fields. Empty body clears it; empty labels/assignees clear their sets. Omitted fields are preserved. GitHub provides no atomic revision condition for these edits."))
	declared.add(command[wire.CreateCommentInput, wire.Comment]("create-issue-comment", "Create a discussion comment on an issue or PR; code-line comments use create_pull_request_comment."))
	declared.add(command[wire.UpdateCommentInput, wire.Comment]("update-issue-comment", "Replace one issue/PR discussion comment's body by its inspected comment ID."))
	declared.add(command[wire.CommentInput, wire.DeletedComment]("delete-issue-comment", "Delete one issue/PR discussion comment by its inspected ID. This does not delete an inline code-review comment."))
	declared.add(command[wire.CreatePullInput, wire.PullRequest]("create-pull-request", "Create a PR from existing head/base branches, optionally as a draft. This does not create commits or push a branch."))
	declared.add(command[wire.SubmitReviewInput, wire.Review]("submit-pull-request-review", "Create and submit an APPROVE, REQUEST_CHANGES or COMMENT review against the explicit inspected commit, optionally with inline comments. Body is required for COMMENT and REQUEST_CHANGES. Does not leave a pending review."))
	declared.add(command[wire.MergePullInput, wire.MergeResult]("merge-pull-request", "Directly merge a PR with the explicit merge method and inspected head SHA. GitHub enforces protection and permissions. Merge queues and stacked PRs require GitHub's separate asynchronous API and are not supported by this operation."))
	declared.add(command[wire.CreateReviewCommentInput, wire.ReviewComment]("create-pull-request-comment", "Create an inline PR review comment at the inspected commit/path/line/side; optional startLine/startSide defines a range."))
	declared.add(command[wire.ReplyReviewCommentInput, wire.ReviewComment]("reply-pull-request-comment", "Reply to an existing inline PR review comment by its inspected comment ID, preserving its upstream thread."))
	declared.add(command[wire.UpdateCommentInput, wire.ReviewComment]("update-pull-request-comment", "Replace an inline PR review comment's body by its inspected comment ID."))
	declared.add(command[wire.CommentInput, wire.DeletedComment]("delete-pull-request-comment", "Delete an inline PR review comment by its inspected comment ID."))
	if err := errors.Join(declared.errs...); err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.github.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, declared.methods...)
}

type declarations struct {
	methods []frame.MethodDescriptor
	errs    []error
}

func (d *declarations) add(method frame.MethodDescriptor, err error) {
	if err != nil {
		d.errs = append(d.errs, err)
		return
	}
	d.methods = append(d.methods, method)
}

func query[P, R any](name, description string) (frame.MethodDescriptor, error) {
	method, err := pluginmcp.NewMethod[P, R](frame.MethodConfig{Name: methodName(name), Kind: frame.Query, Target: extension.ScopeInstallation, Description: description})
	if err != nil {
		return frame.MethodDescriptor{}, err
	}
	return method.Descriptor(), nil
}

func command[P, R any](name, description string) (frame.MethodDescriptor, error) {
	method, err := pluginmcp.NewMethod[P, R](frame.MethodConfig{Name: methodName(name), Kind: frame.Command, Target: extension.ScopeInstallation, Description: description + " Requires the user's intent and a configured token. Recovery reads the original receipt; an unknown outcome never permits resubmission."})
	if err != nil {
		return frame.MethodDescriptor{}, err
	}
	return method.Descriptor(), nil
}

func methodName(name string) string { return "flame.github/" + name + ".v1" }
