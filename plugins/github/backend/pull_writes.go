package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
)

var commitSHA = regexp.MustCompile(contractwire.CommitPattern)

func (c *connector) createPull(ctx context.Context, input contractwire.CreatePullInput) (contractwire.PullRequest, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.PullRequest{}, err
	}
	if !textPresent(input.Title) || !textPresent(input.Head) || !textPresent(input.Base) || input.HeadRepository != nil && !textPresent(*input.HeadRepository) {
		return contractwire.PullRequest{}, refused("invalid_input", "PR title, head and base must be nonblank")
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
	return writeValue(ctx, c, http.MethodPost, repository.apiPath()+"/pulls", payload, http.StatusCreated, func(wire pullWire) (contractwire.PullRequest, error) {
		if input.MaintainerCanModify != nil && wire.MaintainerCanModify == nil {
			return contractwire.PullRequest{}, errors.New("github: PR acknowledgement omits maintainer permissions")
		}
		return wire.admit(repository)
	})
}

func (c *connector) submitReview(ctx context.Context, input contractwire.SubmitReviewInput) (contractwire.Review, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.Review{}, err
	}
	if !commitSHA.MatchString(input.CommitID) {
		return contractwire.Review{}, refused("invalid_input", "an exact inspected review commit SHA is required")
	}
	var expectedState string
	switch input.Event {
	case contractwire.Approve:
		expectedState = "APPROVED"
	case contractwire.RequestChanges:
		expectedState = "CHANGES_REQUESTED"
	case contractwire.CommentReview:
		expectedState = "COMMENTED"
	default:
		return contractwire.Review{}, refused("invalid_input", "a submitted review event is required")
	}
	if input.Event != contractwire.Approve && (input.Body == nil || !textPresent(*input.Body)) {
		return contractwire.Review{}, refused("invalid_input", "COMMENT and REQUEST_CHANGES reviews require a nonblank body")
	}
	comments := make([]inlineCommentWire, len(input.Comments))
	for i, comment := range input.Comments {
		comments[i], err = inlineComment(comment)
		if err != nil {
			return contractwire.Review{}, err
		}
	}
	payload := struct {
		CommitID string                   `json:"commit_id"`
		Event    contractwire.ReviewEvent `json:"event"`
		Body     *string                  `json:"body,omitzero"`
		Comments []inlineCommentWire      `json:"comments,omitzero"`
	}{strings.ToLower(input.CommitID), input.Event, input.Body, comments}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire reviewWire) (contractwire.Review, error) {
		if wire.CommitID == nil || !strings.EqualFold(*wire.CommitID, input.CommitID) || wire.State != expectedState || wire.SubmittedAt == nil || wire.SubmittedAt.IsZero() {
			return contractwire.Review{}, errors.New("github: review acknowledgement differs from submitted commit or decision")
		}
		return wire.admit()
	})
}

func (c *connector) mergePull(ctx context.Context, input contractwire.MergePullInput) (contractwire.MergeResult, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.MergeResult{}, err
	}
	if !commitSHA.MatchString(input.HeadSHA) {
		return contractwire.MergeResult{}, refused("invalid_input", "an exact inspected head SHA is required for merge")
	}
	if input.MergeMethod != contractwire.Merge && input.MergeMethod != contractwire.Squash && input.MergeMethod != contractwire.Rebase {
		return contractwire.MergeResult{}, refused("invalid_input", "mergeMethod must be merge, squash or rebase")
	}
	payload := struct {
		SHA     string                   `json:"sha"`
		Method  contractwire.MergeMethod `json:"merge_method"`
		Title   *string                  `json:"commit_title,omitzero"`
		Message *string                  `json:"commit_message,omitzero"`
	}{strings.ToLower(input.HeadSHA), input.MergeMethod, input.CommitTitle, input.CommitMessage}
	return writeValue(ctx, c, http.MethodPut, fmt.Sprintf("%s/pulls/%d/merge", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire contractwire.MergeResult) (contractwire.MergeResult, error) {
		if !wire.Merged || !commitSHA.MatchString(wire.SHA) {
			return contractwire.MergeResult{}, errors.New("github: merge acknowledgement does not confirm a merge commit")
		}
		return wire, nil
	})
}
