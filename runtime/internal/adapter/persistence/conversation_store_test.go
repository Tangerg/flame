package persistence

import (
	"context"
	"errors"
	"testing"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/frame/sqlite"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

type stubConversationMessages struct {
	err error
}

func (s stubConversationMessages) Read(context.Context, history.ConversationID) ([]chat.Message, error) {
	return nil, nil
}

func (s stubConversationMessages) Write(
	context.Context,
	history.ConversationID,
	...chat.Message,
) error {
	return s.err
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
		err       error
		uncertain bool
	}{
		"rejected batch": {err: cause},
		"unknown commit": {err: errors.Join(cause, sqlite.ErrIndeterminate), uncertain: true},
	} {
		t.Run(name, func(t *testing.T) {
			store, err := NewConversationStore(stubConversationMessages{err: testCase.err})
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

func TestConversationStorePreservesCommittedSuccess(t *testing.T) {
	store, err := NewConversationStore(stubConversationMessages{})
	if err != nil {
		t.Fatalf("NewConversationStore: %v", err)
	}
	if writeErr := store.Write(t.Context(), "ses_1", chat.NewUserMessage(chat.NewTextPart("hi"))); writeErr != nil {
		t.Fatalf("committed batch reported as failed: %v", writeErr)
	}
}
