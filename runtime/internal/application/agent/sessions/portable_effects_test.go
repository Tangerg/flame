package sessions

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestPortableSnapshotPreservesUnresolvedEffectsAsTerminalHistory(t *testing.T) {
	effect, err := run.NewUnresolvedEffect("process_source", "effect_source", "canceled", "owner stopped", "external result is unconfirmed")
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []run.Outcome{
		run.OutcomeCompleted, run.OutcomeCanceled, run.OutcomeTimedOut, run.OutcomeFailed, run.OutcomeLost,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			source := portableSnapshot()
			value := source.Runs[0].Snapshot()
			value.State, _ = run.Running.Terminate(outcome)
			value.Outcome = &outcome
			value.UnresolvedEffects = []run.UnresolvedEffect{effect}
			switch outcome {
			case run.OutcomeTimedOut:
				value.Failure = &run.Failure{Kind: run.FailureTimeout}
			case run.OutcomeFailed:
				value.Failure = &run.Failure{Kind: run.FailureProviderUnavailable}
			case run.OutcomeLost:
				value.Failure = &run.Failure{Kind: run.FailureLost}
			}
			source.Runs[0] = testsupport.MustRestoreRun(value)

			portable, err := source.PortableSnapshot()
			if err != nil {
				t.Fatalf("PortableSnapshot: %v", err)
			}
			if !slices.Equal(portable.Runs[0].UnresolvedEffects, []run.UnresolvedEffect{effect}) {
				t.Fatalf("portable effects = %+v, want source evidence", portable.Runs[0].UnresolvedEffects)
			}
			restored, err := portable.CanonicalSnapshot()
			if err != nil {
				t.Fatalf("CanonicalSnapshot: %v", err)
			}
			if !source.Runs[0].Equal(restored.Runs[0]) {
				t.Fatalf("restored run = %+v, want complete source record %+v", restored.Runs[0].Snapshot(), source.Runs[0].Snapshot())
			}
			if _, err := restored.Runs[0].Resume("seg_attempt", time.Unix(3, 0)); err == nil {
				t.Fatal("imported historical evidence made a terminal Run resumable")
			}

			portable.Runs[0].UnresolvedEffects[0] = run.UnresolvedEffect{}
			if source.Runs[0].UnresolvedEffects()[0] != effect || restored.Runs[0].UnresolvedEffects()[0] != effect {
				t.Fatal("portable evidence mutation changed the source or restored Run")
			}
		})
	}
}

func TestPortableSnapshotRejectsInvalidUnresolvedEffects(t *testing.T) {
	effect, err := run.NewUnresolvedEffect("process_source", "effect_source", "canceled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for name, effects := range map[string][]run.UnresolvedEffect{
		"invalid value":      {{}},
		"duplicate identity": {effect, effect},
	} {
		t.Run(name, func(t *testing.T) {
			portable, err := portableSnapshot().PortableSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			portable.Runs[0].UnresolvedEffects = effects
			if _, err := portable.CanonicalSnapshot(); !errors.Is(err, ErrInvalidPortableSnapshot) {
				t.Fatalf("CanonicalSnapshot error = %v, want invalid evidence rejection", err)
			}
		})
	}
}
