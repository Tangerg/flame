package transport_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/sqlite"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/transport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestBackendMinesAndPublishesOnlyItsPrivateMemoryWithBoundedHostModels(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewAgentMemoryStore(db)
	roots, err := workspaceapp.NewScope("", "", workspace.Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "memory"}, nil)
	if err := transport.RegisterServer(server, db, store, roots); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime"}, nil)
	if err := rpc.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	project, err := (workspace.Resolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	broker := managedmcp.NewModels()
	broker.AdmitSession(session)
	operations := []string{}
	grant, release, err := broker.Grant(t.Context(), session, func(_ context.Context, prompt managedmcp.Prompt) (string, error) {
		operations = append(operations, prompt.Operation)
		if prompt.Operation == "memory.extract" {
			return "- use local workspace mode\n- verify backend restart", nil
		}
		if prompt.Operation == "memory.curate" && strings.Contains(prompt.UserPrompt, "use local workspace mode") {
			return "- use local workspace mode\n- verify backend restart", nil
		}
		t.Errorf("unexpected model request: %s", prompt.Operation)
		return "", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = managedmcp.Call[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: true, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(operations, ",") != "memory.extract,memory.curate" {
		t.Fatalf("model lifecycle=%v", operations)
	}
	state, err := store.State(t.Context(), project)
	if err != nil || state.Watermark != 2 {
		t.Fatalf("private generation=%+v %v", state, err)
	}
	page, err := managedmcp.Call[rpc.ListInput, rpc.Page](t.Context(), session, rpc.List, rpc.ListInput{Scope: memory.ScopeProject, CWD: project})
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("published proposals=%+v %v", page, err)
	}
	for _, item := range page.Rows {
		if item.Status != memory.StatusPending || item.Origin != memory.OriginAuto {
			t.Fatalf("proposal skipped human review=%+v", item)
		}
	}
	release()

	_, err = managedmcp.Call[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: false, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4}})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.State(t.Context(), project)
	if err != nil || unchanged != state {
		t.Fatalf("non-compaction advanced memory=%+v %v", unchanged, err)
	}
}
