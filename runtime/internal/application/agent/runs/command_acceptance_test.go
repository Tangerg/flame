package runs

import (
	"context"
	"strings"
	"testing"
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
		Found: true, MaterialAvailable: true, Acceptance: validCommandAcceptance(),
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
}

func TestCommandAcceptanceValidateRejectsEachMissingFact(t *testing.T) {
	cases := map[string]func(*CommandAcceptance){
		"empty key":          func(a *CommandAcceptance) { a.Key = "" },
		"short fingerprint":  func(a *CommandAcceptance) { a.Fingerprint = strings.Repeat("a", 63) },
		"uppercase digest":   func(a *CommandAcceptance) { a.Fingerprint = strings.Repeat("A", 64) },
		"unknown operation":  func(a *CommandAcceptance) { a.Operation = "resume" },
		"absent operation":   func(a *CommandAcceptance) { a.Operation = "" },
		"empty session":      func(a *CommandAcceptance) { a.SessionID = "" },
		"empty run":          func(a *CommandAcceptance) { a.RunID = "" },
		"empty segment":      func(a *CommandAcceptance) { a.SegmentID = "" },
		"empty user item":    func(a *CommandAcceptance) { a.UserItemID = "" },
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
