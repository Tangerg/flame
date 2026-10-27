package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

func textPresent(value string) bool { return utf8.ValidString(value) && strings.TrimSpace(value) != "" }

func targetRepository(input rpc.IssueInput) (repository, error) {
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

func (c *connector) createIssue(ctx context.Context, input rpc.CreateIssueInput) (rpc.Issue, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.Issue{}, err
	}
	if !textPresent(input.Title) {
		return rpc.Issue{}, refused("invalid_input", "issue title must be nonblank")
	}
	if err := metadataNames(input.Labels); err != nil {
		return rpc.Issue{}, err
	}
	if err := metadataNames(input.Assignees); err != nil {
		return rpc.Issue{}, err
	}
	payload := struct {
		Title     string   `json:"title"`
		Body      *string  `json:"body,omitzero"`
		Labels    []string `json:"labels,omitzero"`
		Assignees []string `json:"assignees,omitzero"`
	}{input.Title, input.Body, input.Labels, input.Assignees}
	return writeValue(ctx, c, http.MethodPost, repository.apiPath()+"/issues", payload, http.StatusCreated, func(wire issueWire) (rpc.Issue, error) { return wire.admit(repository) })
}

func (c *connector) updateIssue(ctx context.Context, input rpc.UpdateIssueInput) (rpc.Issue, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.Issue{}, err
	}
	if input.Title == nil && input.Body == nil && input.State == nil && input.StateReason == nil && input.Labels == nil && input.Assignees == nil && input.DuplicateIssueID == nil {
		return rpc.Issue{}, refused("invalid_input", "an issue edit must supply at least one field")
	}
	if input.Title != nil && !textPresent(*input.Title) {
		return rpc.Issue{}, refused("invalid_input", "issue title must be nonblank")
	}
	if input.State != nil && *input.State != rpc.Open && *input.State != rpc.Closed {
		return rpc.Issue{}, refused("invalid_input", "issue state must be open or closed")
	}
	if input.StateReason != nil {
		if input.State == nil {
			return rpc.Issue{}, refused("invalid_input", "stateReason requires an explicit state change")
		}
		switch *input.StateReason {
		case rpc.Completed, rpc.NotPlanned, rpc.Duplicate:
			if *input.State != rpc.Closed {
				return rpc.Issue{}, refused("invalid_input", "a closing reason requires closed state")
			}
		case rpc.Reopened:
			if *input.State != rpc.Open {
				return rpc.Issue{}, refused("invalid_input", "reopened requires open state")
			}
		default:
			return rpc.Issue{}, refused("invalid_input", "unknown issue state reason")
		}
	}
	duplicate := input.StateReason != nil && *input.StateReason == rpc.Duplicate
	if duplicate != (input.DuplicateIssueID != nil) || input.DuplicateIssueID != nil && *input.DuplicateIssueID <= 0 {
		return rpc.Issue{}, refused("invalid_input", "duplicate reason requires a positive canonical issue ID and no other reason accepts it")
	}
	if input.Labels != nil {
		if err := metadataNames(*input.Labels); err != nil {
			return rpc.Issue{}, err
		}
	}
	if input.Assignees != nil {
		if err := metadataNames(*input.Assignees); err != nil {
			return rpc.Issue{}, err
		}
	}
	payload := struct {
		Title            *string          `json:"title,omitzero"`
		Body             *string          `json:"body,omitzero"`
		State            *rpc.State       `json:"state,omitzero"`
		StateReason      *rpc.StateReason `json:"state_reason,omitzero"`
		DuplicateIssueID *int64           `json:"duplicate_issue_id,omitzero"`
		Labels           *[]string        `json:"labels,omitzero"`
		Assignees        *[]string        `json:"assignees,omitzero"`
	}{input.Title, input.Body, input.State, input.StateReason, input.DuplicateIssueID, input.Labels, input.Assignees}
	return writeValue(ctx, c, http.MethodPatch, fmt.Sprintf("%s/issues/%d", repository.apiPath(), input.Number), payload, http.StatusOK, func(wire issueWire) (rpc.Issue, error) {
		if wire.Number != input.Number {
			return rpc.Issue{}, fmt.Errorf("github: updated issue number differs from original target")
		}
		return wire.admit(repository)
	})
}
