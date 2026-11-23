package bootstrap

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestSessionIsolationIsReachableThroughTheProtocol(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	ctx := protocolLifecycleContext(t.Context())

	instance, api := openProtocolRuntime(t, newReplyStub("ok"))
	t.Cleanup(func() {
		if err := instance.Close(); err != nil {
			t.Error(err)
		}
	})

	created, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "isolation",
	})
	if err != nil {
		t.Fatalf("sessions.create: %v", err)
	}
	if created.Isolated {
		t.Fatal("a fresh session already runs in a scratch copy")
	}

	isolate := true
	updated, err := api.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: created.ID, ExpectedRevision: created.Revision, Isolated: &isolate,
	})
	if err != nil {
		t.Fatalf("sessions.update: %v", err)
	}
	if !updated.Isolated {
		t.Fatalf("updated session = %+v, want isolation on", updated)
	}

	reread, err := api.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("sessions.get: %v", err)
	}
	if !reread.Isolated {
		t.Fatalf("re-read session = %+v, want isolation to be durable", reread)
	}

	release := false
	restored, err := api.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: created.ID, ExpectedRevision: reread.Revision, Isolated: &release,
	})
	if err != nil {
		t.Fatalf("sessions.update releasing isolation: %v", err)
	}
	if restored.Isolated {
		t.Fatalf("restored session = %+v, want isolation off", restored)
	}
}
