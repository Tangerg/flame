package catalog

import (
	"errors"
	"fmt"
	"slices"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

var ErrUnsupportedReasoningEffort = errors.New("model: unsupported reasoning effort")

func (a Admission) admitReasoning() error {
	selection := a.selection
	effort := selection.ReasoningEffort()
	if effort == "" || a.metadata == nil {
		return nil
	}
	entry := a.metadata
	if entry.Reasoning.Supported && slices.Contains(entry.Reasoning.Levels, effort) {
		return nil
	}
	return fmt.Errorf(
		"%w: %w: model %q/%q does not advertise %q (available: %v)",
		flamemodel.ErrUnsupported,
		ErrUnsupportedReasoningEffort,
		selection.Provider(),
		selection.Model(),
		effort,
		entry.Reasoning.Levels,
	)
}
