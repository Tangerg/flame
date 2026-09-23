package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type stubConversationMessages struct {
	outcome history.WriteOutcome
	err     error
}

func (s stubConversationMessages) Read(context.Context, history.ConversationID) ([]chat.Message, error) {
	return nil, nil
}

func (s stubConversationMessages) Write(
	context.Context,
	history.ConversationID,
	...chat.Message,
) (history.WriteOutcome, error) {
	return s.outcome, s.err
}

func (s stubConversationMessages) Clear(context.Context, history.ConversationID) error { return nil }

func (s stubConversationMessages) Count(context.Context, history.ConversationID) (int, error) {
	return 0, nil
}

func (s stubConversationMessages) Replace(context.Context, history.ConversationID, ...chat.Message) error {
	return nil
}

func (s stubConversationMessages) Truncate(context.Context, history.ConversationID, int) error {
	return nil
}

func TestConversationStoreSeparatesRejectedFromUnknownWrites(t *testing.T) {
	cause := errors.New("commit interrupted")
	message := chat.NewUserMessage(chat.NewTextPart("hi"))

	for name, testCase := range map[string]struct {
		outcome   history.WriteOutcome
		uncertain bool
	}{
		"rejected batch": {outcome: history.WriteOutcome{}},
		"unknown commit": {outcome: history.WriteOutcome{Uncertain: true}, uncertain: true},
	} {
		t.Run(name, func(t *testing.T) {
			store, err := persistence.NewConversationStore(stubConversationMessages{outcome: testCase.outcome, err: cause})
			if err != nil {
				t.Fatalf("NewConversationStore: %v", err)
			}
			writeErr := store.Write(t.Context(), "ses_1", message)
			if !errors.Is(writeErr, cause) {
				t.Fatalf("write error = %v, want the store's cause", writeErr)
			}
			if got := errors.Is(writeErr, runsapp.ErrConversationWriteUncertain); got != testCase.uncertain {
				t.Fatalf("uncertain = %t, want %t (%v)", got, testCase.uncertain, writeErr)
			}
		})
	}
}

func TestConversationStoreRejectsAnUnaccountedSuccess(t *testing.T) {
	store, err := persistence.NewConversationStore(stubConversationMessages{})
	if err != nil {
		t.Fatalf("NewConversationStore: %v", err)
	}
	if writeErr := store.Write(t.Context(), "ses_1", chat.NewUserMessage(chat.NewTextPart("hi"))); writeErr == nil {
		t.Fatal("a success that acknowledged no message was reported as a complete write")
	}
}
