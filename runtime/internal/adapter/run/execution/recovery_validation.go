package execution

import (
	"context"
	"errors"
	"fmt"
	"iter"

	scoperun "github.com/Tangerg/scope/agent/run"
)

var errRecoveryValidated = errors.New("execution: recovery validated before writer activation")

// Recover validates strategy restoration and deterministic replay before it
// acquires a writer. This read-only store stops at that public boundary; a
// probe never publishes a Process, dispatches an Effect, or changes storage.
func validateRecovery(ctx context.Context, program scoperun.Program[scoperun.Payload, scoperun.Payload], history scoperun.Log) (resultErr error) {
	owned, err := history.Clone()
	if err != nil {
		return err
	}
	probe, err := scoperun.New(ctx, scoperun.Config{Store: recoveryProbeStore{history: owned}, Build: program.ProgramRef().Build()})
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, probe.Close(context.WithoutCancel(ctx))) }()
	_, err = probe.Recover(ctx, program, history.Root)
	if errors.Is(err, errRecoveryValidated) {
		return nil
	}
	if err == nil {
		return errors.New("execution: read-only recovery unexpectedly acquired a writer")
	}
	return err
}

type recoveryProbeStore struct{ history scoperun.Log }

func (s recoveryProbeStore) Load(ctx context.Context, root scoperun.ProcessID) (scoperun.Log, bool, error) {
	if err := ctx.Err(); err != nil {
		return scoperun.Log{}, false, err
	}
	if root != s.history.Root {
		return scoperun.Log{}, false, nil
	}
	owned, err := s.history.Clone()
	return owned, err == nil, err
}

func (s recoveryProbeStore) Read(ctx context.Context, root scoperun.ProcessID, after scoperun.LSN) iter.Seq2[scoperun.Entry, error] {
	return func(yield func(scoperun.Entry, error) bool) {
		if root != s.history.Root {
			return
		}
		for _, entry := range s.history.Entries {
			if entry.LSN <= after {
				continue
			}
			if err := ctx.Err(); err != nil {
				yield(scoperun.Entry{}, err)
				return
			}
			if !yield(entry, nil) {
				return
			}
		}
	}
}

func (s recoveryProbeStore) Append(ctx context.Context, batch scoperun.Batch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if batch.Root != s.history.Root || len(batch.Records) != 1 {
		return errors.New("execution: recovery probe attempted to write execution work")
	}
	activation, ok := batch.Records[0].(scoperun.Activated)
	if !ok || activation.Epoch != s.history.Epoch()+1 {
		return errors.New("execution: recovery probe attempted an invalid activation")
	}
	if _, retry, err := batch.Admit(s.history.Epoch(), s.history.Head(), nil); err != nil {
		return fmt.Errorf("execution: recovery probe activation: %w", err)
	} else if retry {
		return errors.New("execution: recovery probe attempted to repeat activation")
	}
	return errRecoveryValidated
}
