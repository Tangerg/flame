package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

var commitSHA = regexp.MustCompile(rpc.CommitPattern)

func (c *connector) createPull(ctx context.Context, input rpc.CreatePullInput) (rpc.PullRequest, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.PullRequest{}, err
	}
	if !textPresent(input.Title) || !textPresent(input.Head) || !textPresent(input.Base) || input.HeadRepository != nil && !textPresent(*input.HeadRepository) {
		return rpc.PullRequest{}, refused("invalid_input", "PR title, head and base must be nonblank")
	}
	payload := struct {
		Title               string  `json:"title"`
		Head                string  `json:"head"`
		Base                string  `json:"base"`
		HeadRepository      *string `json:"head_repo,omitzero"`
		Body                *string `json:"body,omitzero"`
		Draft               *bool   `json:"draft,omitzero"`
		MaintainerCanModify *bool   `json:"maintainer_can_modify,omitzero"`
	}{input.Title, input.Head, input.Base, input.HeadRepository, input.Body, input.Draft, input.MaintainerCanModify}
	return writeValue(ctx, c, http.MethodPost, repository.apiPath()+"/pulls", payload, http.StatusCreated, func(wire pullWire) (rpc.PullRequest, error) {
		if input.MaintainerCanModify != nil && wire.MaintainerCanModify == nil {
			return rpc.PullRequest{}, errors.New("github: PR acknowledgement omits maintainer permissions")
		}
		return wire.admit(repository)
	})
}

func (c *connector) submitReview(ctx context.Context, input rpc.SubmitReviewInput) (rpc.Review, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.Review{}, err
	}
	if !commitSHA.MatchString(input.CommitID) {
		return rpc.Review{}, refused("invalid_input", "an exact inspected review commit SHA is required")
	}
	var expectedState string
	switch input.Event {
	case rpc.Approve:
		expectedState = "APPROVED"
	case rpc.RequestChanges:
		expectedState = "CHANGES_REQUESTED"
	case rpc.CommentReview:
		expectedState = "COMMENTED"
	default:
		return rpc.Review{}, refused("invalid_input", "a submitted review event is required")
	}
	if input.Event != rpc.Approve && (input.Body == nil || !textPresent(*input.Body)) {
		return rpc.Review{}, refused("invalid_input", "COMMENT and REQUEST_CHANGES reviews require a nonblank body")
	}
	comments := make([]inlineCommentWire, len(input.Comments))
	for i, comment := range input.Comments {
		comments[i], err = inlineComment(comment)
		if err != nil {
			return rpc.Review{}, err
		}
	}
	payload := struct {
		CommitID string              `json:"commit_id"`
		Event    rpc.ReviewEvent     `json:"event"`
		Body     *string             `json:"body,omitzero"`
		Comments []inlineCommentWire `json:"comments,omitzero"`
	}{strings.ToLower(input.CommitID), input.Event, input.Body, comments}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire reviewWire) (rpc.Review, error) {
		if wire.CommitID == nil || !strings.EqualFold(*wire.CommitID, input.CommitID) || wire.State != expectedState || wire.SubmittedAt == nil || wire.SubmittedAt.IsZero() {
			return rpc.Review{}, errors.New("github: review acknowledgement differs from submitted commit or decision")
		}
		return wire.admit()
	})
}

func (c *connector) mergePull(ctx context.Context, input rpc.MergePullInput) (rpc.MergeResult, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.MergeResult{}, err
	}
	if !commitSHA.MatchString(input.HeadSHA) {
		return rpc.MergeResult{}, refused("invalid_input", "an exact inspected head SHA is required for merge")
	}
	if input.MergeMethod != rpc.Merge && input.MergeMethod != rpc.Squash && input.MergeMethod != rpc.Rebase {
		return rpc.MergeResult{}, refused("invalid_input", "mergeMethod must be merge, squash or rebase")
	}
	payload := struct {
		SHA     string          `json:"sha"`
		Method  rpc.MergeMethod `json:"merge_method"`
		Title   *string         `json:"commit_title,omitzero"`
		Message *string         `json:"commit_message,omitzero"`
	}{strings.ToLower(input.HeadSHA), input.MergeMethod, input.CommitTitle, input.CommitMessage}
	return writeValue(ctx, c, http.MethodPut, fmt.Sprintf("%s/pulls/%d/merge", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire rpc.MergeResult) (rpc.MergeResult, error) {
		if !wire.Merged || !commitSHA.MatchString(wire.SHA) {
			return rpc.MergeResult{}, errors.New("github: merge acknowledgement does not confirm a merge commit")
		}
		return wire, nil
	})
}
