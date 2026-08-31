package agent

import "testing"

func TestSignalRequiresContent(t *testing.T) {
	if err := (FeedbackSignal{SessionID: "ses_1", Rating: FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("rated signal: %v", err)
	}
	if err := (FeedbackSignal{SessionID: "ses_1", Text: "details"}).Validate(); err != nil {
		t.Fatalf("text signal: %v", err)
	}
	if err := (FeedbackSignal{SessionID: "ses_1"}).Validate(); err == nil {
		t.Fatal("accepted empty signal")
	}
}

func TestSignalOwnsExactOptionalTargetIdentities(t *testing.T) {
	tests := []struct {
		name   string
		signal FeedbackSignal
	}{
		{name: "non-exact session", signal: FeedbackSignal{SessionID: " ses_1", Rating: FeedbackPositive}},
		{name: "non-exact run", signal: FeedbackSignal{SessionID: "ses_1", RunID: "run_1 ", Rating: FeedbackPositive}},
		{name: "non-exact item", signal: FeedbackSignal{SessionID: "ses_1", ItemID: " item_1", Rating: FeedbackPositive}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.signal.Validate(); err == nil {
				t.Fatalf("invalid signal was accepted: %+v", test.signal)
			}
		})
	}

	valid := FeedbackSignal{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Rating: FeedbackPositive}
	if err := valid.Validate(); err != nil {
		t.Fatalf("fully targeted signal: %v", err)
	}
	if err := (FeedbackSignal{Rating: FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("untargeted signal: %v", err)
	}
	if err := (FeedbackSignal{ItemID: "item_1", Rating: FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("item-only signal: %v", err)
	}
}
