package agentmemory

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The target, watermark and contents stay bound so persistence cannot mix decisions from separate folds.
type Publication struct {
	project  string
	expected State
	state    State
	contents []string
}

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

func (p Publication) Project() string { return p.project }

func (p Publication) ExpectedState() State { return p.expected }

func (p Publication) State() State { return p.state }

func (p Publication) Contents() []string { return slices.Clone(p.contents) }

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
