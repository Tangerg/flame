package runs

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

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

// ResultPublicationLookup asks the Segment owner to verify an exact durable
// receipt before the executor reconstructs product metadata for a replay.
// It is ordered with commits on the same stream and cannot advance a reducer.
type ResultPublicationLookup struct {
	executorPayloadBase
	publication ResultPublication
	state       *resultPublicationLookupState
}

type resultPublicationLookupState struct {
	once      sync.Once
	done      chan struct{}
	committed bool
	err       error
}

func NewResultPublicationLookup(publication ResultPublication) (ResultPublicationLookup, error) {
	if err := publication.Validate(); err != nil {
		return ResultPublicationLookup{}, err
	}
	return ResultPublicationLookup{
		publication: publication,
		state:       &resultPublicationLookupState{done: make(chan struct{})},
	}, nil
}

func (r ResultPublicationLookup) Publication() ResultPublication { return r.publication }

func (r ResultPublicationLookup) validate() error {
	if r.state == nil || r.state.done == nil {
		return errors.New("runs: malformed result publication lookup")
	}
	return r.publication.Validate()
}

func (r ResultPublicationLookup) Complete(committed bool, err error) {
	if r.state == nil {
		return
	}
	r.state.once.Do(func() {
		r.state.committed, r.state.err = committed && err == nil, err
		close(r.state.done)
	})
}

func (r ResultPublicationLookup) Await(ctx context.Context) (bool, error) {
	if err := r.validate(); err != nil {
		return false, err
	}
	select {
	case <-r.state.done:
		return r.state.committed, r.state.err
	case <-ctx.Done():
		return false, context.Cause(ctx)
	}
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
