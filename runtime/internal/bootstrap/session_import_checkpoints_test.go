package bootstrap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSessionImportRetiresCheckpointsFromTheReplacedHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; file checkpoints require git")
	}
	t.Setenv("FLAME_HOME", t.TempDir())
	workspace := t.TempDir()
	if out, err := exec.Command("git", "-C", workspace, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	path := filepath.Join(workspace, "material.txt")
	if err := os.WriteFile(path, []byte("original checkpoint\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := newReplyStub("completed")
	first, api, _ := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "original history",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "record boundary"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "checkpoint boundary")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, api, _ := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	exported, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
	if err != nil || exported.Artifact == nil {
		t.Fatalf("export = %+v, %v", exported, err)
	}
	rollback := protocol.RollbackSessionRequest{
		SessionID: session.ID, ToRunID: started.RunID, RestoreType: protocol.RestoreFiles,
	}
	if _, err := api.RollbackSession(ctx, rollback); err != nil {
		t.Fatalf("original checkpoint is unavailable before import: %v", err)
	}
	if err := os.WriteFile(path, []byte("current material\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exported.Artifact.Session.Title = "restored history"
	imported, err := api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact})
	if err != nil || imported.Session.Title != "restored history" {
		t.Fatalf("import = %+v, %v", imported, err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	third, api, _ := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := third.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := api.RollbackSession(ctx, rollback); !errors.Is(err, protocol.ErrCheckpointUnavailable) {
		t.Fatalf("restored history reused its predecessor's file checkpoint: %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "current material\n" {
		t.Fatalf("workspace content after refused rollback = %q, %v", content, err)
	}
}
