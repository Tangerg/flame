package feedback

import "testing"

func TestSignalRequiresContent(t *testing.T) {
	if err := (Signal{SessionID: "ses_1", Rating: Positive}).Validate(); err != nil {
		t.Fatalf("rated signal: %v", err)
	}
	if err := (Signal{SessionID: "ses_1", Text: "details"}).Validate(); err != nil {
		t.Fatalf("text signal: %v", err)
	}
	if err := (Signal{SessionID: "ses_1"}).Validate(); err == nil {
		t.Fatal("accepted empty signal")
	}
}

func TestSignalOwnsExactOptionalTargetIdentities(t *testing.T) {
	tests := []struct {
		name   string
		signal Signal
	}{
		{name: "non-exact session", signal: Signal{SessionID: " ses_1", Rating: Positive}},
		{name: "non-exact run", signal: Signal{SessionID: "ses_1", RunID: "run_1 ", Rating: Positive}},
		{name: "non-exact item", signal: Signal{SessionID: "ses_1", ItemID: " item_1", Rating: Positive}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.signal.Validate(); err == nil {
				t.Fatalf("invalid signal was accepted: %+v", test.signal)
			}
		})
	}

	valid := Signal{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Rating: Positive}
	if err := valid.Validate(); err != nil {
		t.Fatalf("fully targeted signal: %v", err)
	}
	if err := (Signal{Rating: Positive}).Validate(); err != nil {
		t.Fatalf("untargeted signal: %v", err)
	}
	if err := (Signal{ItemID: "item_1", Rating: Positive}).Validate(); err != nil {
		t.Fatalf("item-only signal: %v", err)
	}
}
