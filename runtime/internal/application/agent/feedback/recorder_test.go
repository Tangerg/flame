package feedback

import (
	"context"
	"errors"
	"testing"

	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/feedback"
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
	recorder, err := NewRecorder(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(t.Context(), Command{ItemID: "item_1", Rating: feedbackdomain.RatingPositive}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(store.entries) != 1 || store.entries[0].ItemID != "item_1" || store.entries[0].CreatedAt.IsZero() {
		t.Fatalf("entries = %+v", store.entries)
	}
}

func TestRecorderRejectsEmptySignalWithoutPersisting(t *testing.T) {
	store := &storeFake{}
	recorder, err := NewRecorder(store)
	if err != nil {
		t.Fatal(err)
	}
	err = recorder.Record(t.Context(), Command{ItemID: "item_1"})
	if !errors.Is(err, feedbackdomain.ErrInvalid) {
		t.Fatalf("Record = %v, want ErrInvalid", err)
	}
	if len(store.entries) != 0 {
		t.Fatalf("entries = %+v, want none", store.entries)
	}
}

func TestNewRecorderRejectsTypedNilStore(t *testing.T) {
	var store *storeFake
	if _, err := NewRecorder(store); err == nil {
		t.Fatal("typed-nil feedback store was accepted")
	}
}

func TestRecorderAcceptsGeneralFeedbackAndPreservesStoreFailure(t *testing.T) {
	wantErr := errors.New("feedback write failed")
	store := &storeFake{err: wantErr}
	recorder, err := NewRecorder(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(t.Context(), Command{Text: "General product feedback"}); !errors.Is(err, wantErr) {
		t.Fatalf("Record = %v, want store failure", err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("received %d entries, want one", len(store.entries))
	}
	entry := store.entries[0]
	if entry.SessionID != "" || entry.RunID != "" || entry.ItemID != "" || entry.CreatedAt.IsZero() {
		t.Fatalf("general feedback gained an execution owner: %+v", entry)
	}
}
