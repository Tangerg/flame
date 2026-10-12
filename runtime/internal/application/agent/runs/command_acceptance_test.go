package runs

import (
	"strings"
	"testing"
)

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
