package runs

import (
	"errors"
	"fmt"

	scoperun "github.com/Tangerg/scope/agent/run"

	"github.com/Tangerg/scope/core/chat"
)

// ResultPublication binds an executor publication to its immutable content.
// Persistence records it atomically with every result; Segment ownership fences
// the writer independently of this stable identity.
type ResultPublication struct {
	ID     string
	Digest string
}

func (r ResultPublication) Validate() error {
	if err := ValidateInvocation(r.ID); err != nil {
		return err
	}
	if _, err := scoperun.ParseDigest(r.Digest); err != nil {
		return fmt.Errorf("runs: result publication digest: %w", err)
	}
	return nil
}

// ToolResultsCommitted carries settled calls with their original round indexes.
// Starts include calls rejected before execution. Existing starts are checked,
// missing starts are projected, and all results enter one transaction.
type ToolResultsCommitted struct {
	executionFactBase
	Publication ResultPublication
	Starts      []ToolCallStarted
	Results     []ToolCallFinished
	// ModelResults is present only when the whole round is known, in declared order.
	ModelResults []chat.ToolResult
}

// validate checks the batch is one model call's complete result set in its
// declared call order.
func (b ToolResultsCommitted) validate() error {
	if err := b.Publication.Validate(); err != nil {
		return err
	}
	if len(b.Results) == 0 || len(b.Starts) != len(b.Results) {
		return errors.New("runs: result publication requires its complete call set")
	}
	sequence := b.Starts[0].ModelCallSequence
	if sequence == 0 {
		return errors.New("runs: result publication requires model attribution")
	}
	for index, start := range b.Starts {
		ordered := index == 0 || start.ToolCallIndex > b.Starts[index-1].ToolCallIndex
		if start.ModelCallSequence != sequence || !ordered || start.CallID != b.Results[index].CallID {
			return errors.New("runs: result publication call set is not in declared order")
		}
	}
	return nil
}

func (t ToolResultsCommitted) clone() ToolResultsCommitted {
	t.Starts = append([]ToolCallStarted(nil), t.Starts...)
	results := make([]ToolCallFinished, len(t.Results))
	for index, result := range t.Results {
		results[index] = result.clone()
	}
	t.Results = results
	t.ModelResults = append([]chat.ToolResult(nil), t.ModelResults...)
	for index := range t.ModelResults {
		t.ModelResults[index] = t.ModelResults[index].Clone()
	}
	return t
}
