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
		return repository{}, invalidInput("a positive issue or PR number is required")
	}
	return newRepository(input.Repository)
}

func metadataNames(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !textPresent(name) || seen[name] {
			return invalidInput("metadata names must be nonblank and distinct")
		}
		seen[name] = true
	}
	return nil
}

func validateIssueEdit(input contractwire.UpdateIssueInput) error {
	if input.Title == nil && input.Body == nil && input.State == nil && input.StateReason == nil && input.Labels == nil && input.Assignees == nil && input.DuplicateIssueID == nil {
		return invalidInput("an issue edit must supply at least one field")
	}
	if input.Title != nil && !textPresent(*input.Title) {
		return invalidInput("issue title must be nonblank")
	}
	if err := validateIssueStateChange(input.State, input.StateReason, input.DuplicateIssueID); err != nil {
		return err
	}
	for _, names := range []*[]string{input.Labels, input.Assignees} {
		if names == nil {
			continue
		}
		if err := metadataNames(*names); err != nil {
			return err
		}
	}
	return nil
}

// validateIssueStateChange admits only the edits GitHub accepts: a reason
// travels with the state it implies, and only a duplicate closure names the
// canonical issue.
func validateIssueStateChange(state *contractwire.State, reason *contractwire.StateReason, duplicateOf *int64) error {
	if state != nil && !state.Valid() {
		return invalidInput("issue state must be open or closed")
	}
	if reason != nil {
		implied, known := reason.State()
		switch {
		case !known:
			return invalidInput("unknown issue state reason")
		case state == nil:
			return invalidInput("stateReason requires an explicit state change")
		case *state != implied:
			return invalidInput(fmt.Sprintf("state reason %s requires %s state", *reason, implied))
		}
	}
	duplicate := reason != nil && *reason == contractwire.Duplicate
	if duplicate != (duplicateOf != nil) || duplicateOf != nil && *duplicateOf <= 0 {
		return invalidInput("duplicate reason requires a positive canonical issue ID and no other reason accepts it")
	}
	return nil
}

func (c *connector) createIssue(ctx context.Context, input contractwire.CreateIssueInput) (contractwire.Issue, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.Issue{}, err
	}
	if !textPresent(input.Title) {
		return contractwire.Issue{}, invalidInput("issue title must be nonblank")
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
	if err := validateIssueEdit(input); err != nil {
		return contractwire.Issue{}, err
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
