package agentmemory

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

// The target, watermark, observed revisions and contents stay bound so persistence cannot mix separate folds.
type Publication struct {
	project  string
	expected State
	state    State
	contents []string
	basis    map[ItemID]uint64
}

func NewPublication(
	project string,
	expected State,
	through int64,
	observed []Item,
	contents []string,
	now time.Time,
) (Publication, error) {
	normalized, err := normalizeGeneration(contents)
	if err != nil {
		return Publication{}, err
	}
	if len(observed) > MaxVisiblePerTarget+MaxRejectedPerTarget {
		return Publication{}, errors.New("agentmemory: curation basis exceeds target bounds")
	}
	basis := make(map[ItemID]uint64, len(observed))
	for _, item := range observed {
		if err := validateGenerationItem(project, item); err != nil {
			return Publication{}, err
		}
		if _, duplicate := basis[item.id]; duplicate {
			return Publication{}, errors.New("agentmemory: curation basis repeats an item")
		}
		basis[item.id] = item.Revision()
	}
	publication := Publication{
		project: project, expected: expected,
		state:    State{Watermark: through, UpdatedAt: now.UTC()},
		contents: normalized, basis: basis,
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
	normalized, err := normalizeGeneration(p.contents)
	if err != nil {
		return err
	}
	if !slices.Equal(normalized, p.contents) {
		return errors.New("agentmemory: publication contents are not canonical")
	}
	for id, revision := range p.basis {
		if err := id.validate(); err != nil {
			return err
		}
		if _, err := exactint.Restore(revision); err != nil {
			return fmt.Errorf("agentmemory: curation basis revision: %w", err)
		}
	}
	return nil
}

func validateGenerationItem(project string, item Item) error {
	if err := item.validate(); err != nil {
		return fmt.Errorf("agentmemory: generation item: %w", err)
	}
	if item.scope != ScopeProject || item.project != project {
		return errors.New("agentmemory: generation item belongs to another target")
	}
	return nil
}
