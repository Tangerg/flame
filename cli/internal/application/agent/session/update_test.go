package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

type updateWriterStub struct {
	calls  int
	result conversation.Session
}

func (u *updateWriterStub) UpdateSession(context.Context, conversation.UpdateSession) (conversation.Session, error) {
	u.calls++
	return u.result, nil
}

func TestUpdateValidatesTheCommandBeforeMutationAndTheResultAfterward(t *testing.T) {
	writer := &updateWriterStub{}
	if _, err := Update(t.Context(), writer, conversation.UpdateSession{SessionID: "ses_1"}); err == nil {
		t.Fatal("Update accepted an empty mutation")
	}
	if writer.calls != 0 {
		t.Fatalf("invalid command reached the runtime %d time(s)", writer.calls)
	}

	title := "Renamed"
	writer.result = conversation.Session{
		ID: "ses_wrong", Title: title, Status: protocol.SessionStatusIdle, Revision: 2,
		Workspace: workspace.Workspace{Path: "/workspace", ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
	}
	_, err := Update(t.Context(), writer, conversation.UpdateSession{
		SessionID: "ses_1", Title: &title, ExpectedRevision: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime returned session") {
		t.Fatalf("Update result error = %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("valid command reached the runtime %d time(s), want 1", writer.calls)
	}
}
