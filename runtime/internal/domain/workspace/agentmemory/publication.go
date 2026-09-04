package agentmemory

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Publication is one exact curation-generation decision. It binds the target,
// expected watermark, successor state, and complete canonical content set so
// persistence cannot combine fields from different folds.
type Publication struct {
	project  string
	expected State
	state    State
	contents []string
}

// NewPublication constructs one monotonic curation-generation publication.
func NewPublication(
	project string,
	expected State,
	through int64,
	contents []string,
	now time.Time,
) (Publication, error) {
	normalized, err := NormalizeGeneration(contents)
	if err != nil {
		return Publication{}, err
	}
	publication := Publication{
		project: project, expected: expected,
		state:    State{Watermark: through, UpdatedAt: now.UTC()},
		contents: normalized,
	}
	if err := publication.Validate(); err != nil {
		return Publication{}, err
	}
	return publication, nil
}

// Project returns the exact project partition this publication advances.
func (p Publication) Project() string { return p.project }

// ExpectedState returns the curation state this publication was derived from.
func (p Publication) ExpectedState() State { return p.expected }

// State returns the already-decided successor curation state.
func (p Publication) State() State { return p.state }

// Contents returns an owned copy of the complete curated generation.
func (p Publication) Contents() []string { return slices.Clone(p.contents) }

// Validate proves the publication advances one valid project state
// monotonically and owns a canonical bounded generation.
func (p Publication) Validate() error {
	if err := ValidateTarget(ScopeProject, p.project); err != nil {
		return err
	}
	if err := p.expected.Validate(); err != nil {
		return fmt.Errorf("agentmemory: publication expected state: %w", err)
	}
	if err := p.state.Validate(); err != nil {
		return fmt.Errorf("agentmemory: publication state: %w", err)
	}
	if p.state.Watermark <= p.expected.Watermark {
		return fmt.Errorf(
			"agentmemory: publication watermark %d does not follow %d",
			p.state.Watermark,
			p.expected.Watermark,
		)
	}
	if !p.expected.UpdatedAt.IsZero() && p.state.UpdatedAt.Before(p.expected.UpdatedAt) {
		return errors.New("agentmemory: publication update time precedes expected state")
	}
	normalized, err := NormalizeGeneration(p.contents)
	if err != nil {
		return err
	}
	if !slices.Equal(normalized, p.contents) {
		return errors.New("agentmemory: publication contents are not canonical")
	}
	return nil
}
