package catalog

import (
	"errors"
	"fmt"
	"slices"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

// ErrUnsupportedReasoningEffort reports an explicit intensity absent from the
// exact known model's advertised vocabulary.
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
		modelchoice.ErrUnsupported,
		ErrUnsupportedReasoningEffort,
		selection.Provider(),
		selection.Model(),
		effort,
		entry.Reasoning.Levels,
	)
}
