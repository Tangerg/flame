package runs

import (
	"encoding/hex"
	"errors"
	"strings"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ResultPublication binds an executor publication to its immutable content.
// Persistence records it atomically with every result; Segment ownership fences
// the writer independently of this stable identity.
type ResultPublication struct {
	ID     string
	Digest string
}

func (r ResultPublication) Validate() error {
	if err := runtimeidentity.ValidateEffect(r.ID); err != nil {
		return err
	}
	value, found := strings.CutPrefix(r.Digest, "sha256:")
	if !found || len(value) != 64 || strings.ToLower(value) != value {
		return errors.New("runs: invalid result publication digest")
	}
	_, err := hex.DecodeString(value)
	return err
}

// ToolResultsCommitted carries a complete model round in declared call order.
// Starts include calls rejected before execution. Existing starts are checked,
// missing starts are projected, and all results enter one transaction.
type ToolResultsCommitted struct {
	executionFactBase
	Publication ResultPublication
	Starts      []ToolCallStarted
	Results     []ToolCallFinished
}

func (t ToolResultsCommitted) clone() ToolResultsCommitted {
	t.Starts = append([]ToolCallStarted(nil), t.Starts...)
	results := make([]ToolCallFinished, len(t.Results))
	for index, result := range t.Results {
		results[index] = result.clone()
	}
	t.Results = results
	return t
}
