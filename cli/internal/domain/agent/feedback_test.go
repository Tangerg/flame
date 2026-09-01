package agent

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSignalRequiresContent(t *testing.T) {
	if err := (FeedbackSignal{SessionID: "ses_1", Rating: protocol.FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("rated signal: %v", err)
	}
	if err := (FeedbackSignal{SessionID: "ses_1", Text: "details"}).Validate(); err != nil {
		t.Fatalf("text signal: %v", err)
	}
	if err := (FeedbackSignal{SessionID: "ses_1"}).Validate(); err == nil {
		t.Fatal("accepted empty signal")
	}
	if err := (FeedbackSignal{SessionID: "ses_1", Rating: protocol.FeedbackRating("mixed")}).Validate(); err == nil {
		t.Fatal("accepted rating outside the runtime vocabulary")
	}
}

func TestParseFeedbackRatingNormalizesInputIntoRuntimeVocabulary(t *testing.T) {
	rating, err := ParseFeedbackRating("  positive  ")
	if err != nil || rating != protocol.FeedbackPositive {
		t.Fatalf("ParseFeedbackRating = (%q, %v)", rating, err)
	}
	if _, err := ParseFeedbackRating("mixed"); err == nil {
		t.Fatal("accepted rating outside the runtime vocabulary")
	}
}

func TestSignalOwnsExactOptionalTargetIdentities(t *testing.T) {
	tests := []struct {
		name   string
		signal FeedbackSignal
	}{
		{name: "non-exact session", signal: FeedbackSignal{SessionID: " ses_1", Rating: protocol.FeedbackPositive}},
		{name: "non-exact run", signal: FeedbackSignal{SessionID: "ses_1", RunID: "run_1 ", Rating: protocol.FeedbackPositive}},
		{name: "non-exact item", signal: FeedbackSignal{SessionID: "ses_1", ItemID: " item_1", Rating: protocol.FeedbackPositive}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.signal.Validate(); err == nil {
				t.Fatalf("invalid signal was accepted: %+v", test.signal)
			}
		})
	}

	valid := FeedbackSignal{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Rating: protocol.FeedbackPositive}
	if err := valid.Validate(); err != nil {
		t.Fatalf("fully targeted signal: %v", err)
	}
	if err := (FeedbackSignal{Rating: protocol.FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("untargeted signal: %v", err)
	}
	if err := (FeedbackSignal{ItemID: "item_1", Rating: protocol.FeedbackPositive}).Validate(); err != nil {
		t.Fatalf("item-only signal: %v", err)
	}
}
