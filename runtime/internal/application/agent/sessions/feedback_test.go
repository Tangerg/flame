package sessions

import (
	"context"
	"errors"
	"testing"

	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/session/feedback"
)

type storeFake struct {
	entries []feedbackdomain.Entry
	err     error
}

func (s *storeFake) Append(_ context.Context, entry feedbackdomain.Entry) error {
	s.entries = append(s.entries, entry)
	return s.err
}

func TestRecorderPersistsValidatedEntry(t *testing.T) {
	store := &storeFake{}
	recorder, err := NewFeedbackRecorder(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(t.Context(), FeedbackCommand{ItemID: "item_1", Rating: feedbackdomain.RatingPositive}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(store.entries) != 1 || store.entries[0].ItemID != "item_1" || store.entries[0].CreatedAt.IsZero() {
		t.Fatalf("entries = %+v", store.entries)
	}
}

func TestRecorderRejectsEmptySignalWithoutPersisting(t *testing.T) {
	store := &storeFake{}
	recorder, err := NewFeedbackRecorder(store)
	if err != nil {
		t.Fatal(err)
	}
	err = recorder.Record(t.Context(), FeedbackCommand{ItemID: "item_1"})
	if !errors.Is(err, feedbackdomain.ErrInvalid) {
		t.Fatalf("Record = %v, want ErrInvalid", err)
	}
	if len(store.entries) != 0 {
		t.Fatalf("entries = %+v, want none", store.entries)
	}
}

func TestNewFeedbackRecorderRejectsTypedNilStore(t *testing.T) {
	var store *storeFake
	if _, err := NewFeedbackRecorder(store); err == nil {
		t.Fatal("typed-nil feedback store was accepted")
	}
}
