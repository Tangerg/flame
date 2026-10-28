package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
)

func textPresent(value string) bool { return utf8.ValidString(value) && strings.TrimSpace(value) != "" }

func targetRepository(input contractwire.IssueInput) (repository, error) {
	if input.Number <= 0 {
		return repository{}, refused("invalid_input", "a positive issue or PR number is required")
	}
	return newRepository(input.Repository)
}

func metadataNames(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !textPresent(name) || seen[name] {
			return refused("invalid_input", "metadata names must be nonblank and distinct")
		}
		seen[name] = true
	}
	return nil
}

func (c *connector) createIssue(ctx context.Context, input contractwire.CreateIssueInput) (contractwire.Issue, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.Issue{}, err
	}
	if !textPresent(input.Title) {
		return contractwire.Issue{}, refused("invalid_input", "issue title must be nonblank")
	}
	if err := metadataNames(input.Labels); err != nil {
		return contractwire.Issue{}, err
	}
	if err := metadataNames(input.Assignees); err != nil {
		return contractwire.Issue{}, err
	}
	payload := struct {
		Title     string   `json:"title"`
		Body      *string  `json:"body,omitzero"`
		Labels    []string `json:"labels,omitzero"`
		Assignees []string `json:"assignees,omitzero"`
	}{input.Title, input.Body, input.Labels, input.Assignees}
	return writeValue(ctx, c, http.MethodPost, repository.apiPath()+"/issues", payload, http.StatusCreated, func(wire issueWire) (contractwire.Issue, error) { return wire.admit(repository) })
}

func (c *connector) updateIssue(ctx context.Context, input contractwire.UpdateIssueInput) (contractwire.Issue, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.Issue{}, err
	}
	if input.Title == nil && input.Body == nil && input.State == nil && input.StateReason == nil && input.Labels == nil && input.Assignees == nil && input.DuplicateIssueID == nil {
		return contractwire.Issue{}, refused("invalid_input", "an issue edit must supply at least one field")
	}
	if input.Title != nil && !textPresent(*input.Title) {
		return contractwire.Issue{}, refused("invalid_input", "issue title must be nonblank")
	}
	if input.State != nil && *input.State != contractwire.Open && *input.State != contractwire.Closed {
		return contractwire.Issue{}, refused("invalid_input", "issue state must be open or closed")
	}
	if input.StateReason != nil {
		if input.State == nil {
			return contractwire.Issue{}, refused("invalid_input", "stateReason requires an explicit state change")
		}
		switch *input.StateReason {
		case contractwire.Completed, contractwire.NotPlanned, contractwire.Duplicate:
			if *input.State != contractwire.Closed {
				return contractwire.Issue{}, refused("invalid_input", "a closing reason requires closed state")
			}
		case contractwire.Reopened:
			if *input.State != contractwire.Open {
				return contractwire.Issue{}, refused("invalid_input", "reopened requires open state")
			}
		default:
			return contractwire.Issue{}, refused("invalid_input", "unknown issue state reason")
		}
	}
	duplicate := input.StateReason != nil && *input.StateReason == contractwire.Duplicate
	if duplicate != (input.DuplicateIssueID != nil) || input.DuplicateIssueID != nil && *input.DuplicateIssueID <= 0 {
		return contractwire.Issue{}, refused("invalid_input", "duplicate reason requires a positive canonical issue ID and no other reason accepts it")
	}
	if input.Labels != nil {
		if err := metadataNames(*input.Labels); err != nil {
			return contractwire.Issue{}, err
		}
	}
	if input.Assignees != nil {
		if err := metadataNames(*input.Assignees); err != nil {
			return contractwire.Issue{}, err
		}
	}
	payload := struct {
		Title            *string                   `json:"title,omitzero"`
		Body             *string                   `json:"body,omitzero"`
		State            *contractwire.State       `json:"state,omitzero"`
		StateReason      *contractwire.StateReason `json:"state_reason,omitzero"`
		DuplicateIssueID *int64                    `json:"duplicate_issue_id,omitzero"`
		Labels           *[]string                 `json:"labels,omitzero"`
		Assignees        *[]string                 `json:"assignees,omitzero"`
	}{input.Title, input.Body, input.State, input.StateReason, input.DuplicateIssueID, input.Labels, input.Assignees}
	return writeValue(ctx, c, http.MethodPatch, fmt.Sprintf("%s/issues/%d", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire issueWire) (contractwire.Issue, error) {
		if wire.Number != input.Number {
			return contractwire.Issue{}, fmt.Errorf("github: updated issue number differs from original target")
		}
		return wire.admit(repository)
	})
}
