package runs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type recordingAcceptanceReader struct {
	keys   []string
	record CommandAcceptanceRecord
}

func (r *recordingAcceptanceReader) Lookup(_ context.Context, key string) (CommandAcceptanceRecord, error) {
	r.keys = append(r.keys, key)
	return r.record, nil
}

func TestLookupCommandAcceptanceDelegatesNonEmptyKey(t *testing.T) {
	reader := &recordingAcceptanceReader{record: CommandAcceptanceRecord{
		Found: true, CurrentRun: new(run.Run), Acceptance: validCommandAcceptance(),
	}}
	coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader})
	got, err := coordinator.LookupCommandAcceptance(context.Background(), "cmd-key-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !got.Found || got.Acceptance.RunID != "run_1" {
		t.Fatalf("delegated record = %+v, want the reader's record", got)
	}
	if len(reader.keys) != 1 || reader.keys[0] != "cmd-key-1" {
		t.Fatalf("reader keys = %v, want [cmd-key-1]", reader.keys)
	}
}

// TestStartReturnsReopenedForAcceptedKey proves a retried start whose key already
// opened a Run returns that original opening instead of admitting a second one —
// before any Session resolution or admission runs.
func TestStartReturnsReopenedForAcceptedKey(t *testing.T) {
	accepted := validCommandAcceptance()
	reader := &recordingAcceptanceReader{record: CommandAcceptanceRecord{
		Found: true, CurrentRun: new(run.Run), Acceptance: accepted,
	}}
	coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader})
	result, err := coordinator.Start(context.Background(), StartCommand{
		SessionID:  accepted.SessionID,
		Acceptance: &CommandIdentity{Key: accepted.Key, Fingerprint: accepted.Fingerprint},
	})
	if err != nil {
		t.Fatalf("start with accepted key: %v", err)
	}
	if !result.Reopened || result.RunID != accepted.RunID || result.SegmentID != accepted.SegmentID ||
		result.UserItemID != accepted.UserItemID {
		t.Fatalf("start result = %+v, want the reopened original opening", result)
	}
}

// TestStartRejectsAcceptedKeyConflicts proves a key reused for a different request
// or held by another operation refuses rather than reopening the wrong Run.
func TestStartRejectsAcceptedKeyConflicts(t *testing.T) {
	accepted := validCommandAcceptance()
	otherFingerprint := accepted
	otherFingerprint.Fingerprint = strings.Repeat("b", 64)
	cases := map[string]CommandAcceptanceRecord{
		"fingerprint mismatch":       {Found: true, Acceptance: otherFingerprint},
		"bound to another operation": {OtherOperation: true},
	}
	for name, record := range cases {
		t.Run(name, func(t *testing.T) {
			reader := &recordingAcceptanceReader{record: record}
			coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader})
			_, err := coordinator.Start(context.Background(), StartCommand{
				SessionID:  accepted.SessionID,
				Acceptance: &CommandIdentity{Key: accepted.Key, Fingerprint: accepted.Fingerprint},
			})
			if !errors.Is(err, ErrCommandKeyConflict) {
				t.Fatalf("start conflict = %v, want ErrCommandKeyConflict", err)
			}
		})
	}
}

func TestLookupCommandAcceptanceSkipsReaderForEmptyKey(t *testing.T) {
	reader := &recordingAcceptanceReader{record: CommandAcceptanceRecord{Found: true}}
	coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader})
	got, err := coordinator.LookupCommandAcceptance(context.Background(), "")
	if err != nil {
		t.Fatalf("lookup empty: %v", err)
	}
	if got.Found || got.OtherOperation {
		t.Fatalf("empty-key record = %+v, want the not-recorded zero value", got)
	}
	if len(reader.keys) != 0 {
		t.Fatalf("reader consulted for empty key: %v", reader.keys)
	}
}

func validCommandAcceptance() CommandAcceptance {
	return CommandAcceptance{
		Key:         "cmd-key-1",
		Fingerprint: strings.Repeat("a", 64),
		Operation:   CommandStartOpening,
		SessionID:   "ses_1",
		RunID:       "run_1",
		SegmentID:   "seg_1",
		UserItemID:  "item_1",
	}
}

func TestCommandAcceptanceValidateAcceptsCompleteOpening(t *testing.T) {
	if err := validCommandAcceptance().Validate(); err != nil {
		t.Fatalf("valid acceptance rejected: %v", err)
	}
	resume := validCommandAcceptance()
	resume.Operation, resume.UserItemID = CommandResumeOpening, ""
	if err := resume.Validate(); err != nil {
		t.Fatalf("response-only Resume rejected: %v", err)
	}
}

func TestResumeReturnsOriginalAcceptanceBeforeReadingTheRun(t *testing.T) {
	for _, retired := range []bool{false, true} {
		accepted := validCommandAcceptance()
		accepted.Operation, accepted.UserItemID = CommandResumeOpening, ""
		reader := &recordingAcceptanceReader{record: CommandAcceptanceRecord{Found: true, Acceptance: accepted}}
		if !retired {
			reader.record.CurrentRun = new(run.Run)
		}
		coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader})
		result, err := coordinator.Resume(t.Context(), ResumeCommand{RunID: accepted.RunID, Acceptance: &CommandIdentity{Key: accepted.Key, Fingerprint: accepted.Fingerprint}})
		if err != nil || !result.Reopened || result.MaterialRetired != retired || result.SegmentID != accepted.SegmentID || result.UserItemID != "" {
			t.Fatalf("original Resume acceptance=%+v,%v", result, err)
		}
	}
	accepted := validCommandAcceptance()
	coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: &recordingAcceptanceReader{record: CommandAcceptanceRecord{Found: true, Acceptance: accepted}}})
	_, err := coordinator.Resume(t.Context(), ResumeCommand{RunID: accepted.RunID, Acceptance: &CommandIdentity{Key: accepted.Key, Fingerprint: accepted.Fingerprint}})
	if !errors.Is(err, ErrCommandKeyConflict) {
		t.Fatalf("Start key used for Resume=%v", err)
	}
}

func TestCommandAcceptanceValidateRejectsEachMissingFact(t *testing.T) {
	cases := map[string]func(*CommandAcceptance){
		"empty key":         func(a *CommandAcceptance) { a.Key = "" },
		"short fingerprint": func(a *CommandAcceptance) { a.Fingerprint = strings.Repeat("a", 63) },
		"uppercase digest":  func(a *CommandAcceptance) { a.Fingerprint = strings.Repeat("A", 64) },
		"unknown operation": func(a *CommandAcceptance) { a.Operation = "unknown" },
		"absent operation":  func(a *CommandAcceptance) { a.Operation = "" },
		"empty session":     func(a *CommandAcceptance) { a.SessionID = "" },
		"empty run":         func(a *CommandAcceptance) { a.RunID = "" },
		"empty segment":     func(a *CommandAcceptance) { a.SegmentID = "" },
		"empty user item":   func(a *CommandAcceptance) { a.UserItemID = "" },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			acceptance := validCommandAcceptance()
			break_(&acceptance)
			if err := acceptance.Validate(); err == nil {
				t.Fatalf("expected rejection for %q", name)
			}
		})
	}
}
