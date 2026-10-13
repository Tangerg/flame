package transport_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/sqlite"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/transport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestBackendMinesAndPublishesOnlyItsPrivateMemoryWithBoundedHostModels(t *testing.T) {
	store, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	project, err := (workspace.Resolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	operations := []string{}
	grant, release, err := rpc.GrantModels(t.Context(), broker, session, func(_ context.Context, prompt rpc.Prompt) (string, error) {
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
	_, err = pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, "mine", rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: true, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}})
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
	page, err := pluginsdk.Call[rpc.ListInput, rpc.Page](t.Context(), session, rpc.List, rpc.ListInput{Scope: memory.ScopeProject, CWD: project})
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("published proposals=%+v %v", page, err)
	}
	for _, item := range page.Rows {
		if item.Status != memory.StatusPending || item.Origin != memory.OriginAuto {
			t.Fatalf("proposal skipped human review=%+v", item)
		}
	}
	release()

	_, err = pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, "no-compaction", rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: false, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4}})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.State(t.Context(), project)
	if err != nil || unchanged != state {
		t.Fatalf("non-compaction advanced memory=%+v %v", unchanged, err)
	}
}

func connectMemoryBackend(t *testing.T, path string) (*sqlite.AgentMemoryStore, *sdk.ClientSession, func()) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeBackend := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), backend.Close(), db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(closeBackend)
	return store, session, closeBackend
}

func TestRevokedCurationJoinsModelsAndRestoresPendingLedgerWithoutOverwritingHumanFacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "memory.db")
	project, err := (workspace.Resolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, session, closeBackend := connectMemoryBackend(t, path)
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	curating := make(chan struct{})
	joined := make(chan struct{})
	grant, release, err := rpc.GrantModels(ctx, broker, session, func(ctx context.Context, prompt rpc.Prompt) (string, error) {
		if prompt.Operation == "memory.extract" {
			return "- automatic proposal", nil
		}
		if prompt.Operation != "memory.curate" {
			return "", errors.New("unexpected model operation")
		}
		close(curating)
		<-ctx.Done()
		close(joined)
		return "", ctx.Err()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	go func() {
		_, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, "maintenance", rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}})
		result <- err
	}()
	select {
	case <-curating:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	added, err := pluginsdk.CallCommand[rpc.AddInput, rpc.AddResult](ctx, session, rpc.Add, "human-add", rpc.AddInput{Scope: memory.ScopeProject, CWD: project, Content: "human fact"})
	if err != nil {
		t.Fatal(err)
	}
	content, pinned := "human edit during curation", true
	human, err := pluginsdk.CallCommand[rpc.UpdateInput, memory.ItemSnapshot](ctx, session, rpc.Update, "human-edit", rpc.UpdateInput{ID: added.Item.ID.String(), Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	broker.RevokeSession(session)
	select {
	case <-joined:
	default:
		t.Fatal("backend revocation returned before joining model work")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("revoked curation published success")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	state, err := store.State(ctx, project)
	if err != nil || state.Watermark != 0 {
		t.Fatalf("revoked generation=%+v err=%v", state, err)
	}
	pending, err := store.PendingLedger(ctx, project, state.Watermark, memory.MaxLedgerFoldFacts)
	if err != nil || len(pending) != 1 {
		t.Fatalf("revocation lost committed extraction: %+v %v", pending, err)
	}
	closeBackend()
	store, session, _ = connectMemoryBackend(t, path)
	broker.AdmitSession(session)
	originalInput := rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}
	if _, found, err := pluginsdk.ReadReceipt[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, "maintenance", rpc.Maintain, originalInput); err != nil || found {
		t.Fatalf("canceled model work became known receipt: %t, %v", found, err)
	}
	if _, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, "maintenance", originalInput); err == nil {
		t.Fatal("pending original maintenance restarted model work")
	}
	_, err = pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, "uncompacted", rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Grant: grant})
	if err == nil {
		t.Fatal("new backend generation reused a revoked model grant")
	}
	fresh, releaseFresh, err := rpc.GrantModels(ctx, broker, session, func(_ context.Context, prompt rpc.Prompt) (string, error) {
		if prompt.Operation != "memory.curate" {
			t.Errorf("pending recovery repeated extraction: %s", prompt.Operation)
		}
		return "- automatic proposal", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFresh()
	input := rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Grant: fresh}
	published, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, "recover-pending-ledger", input)
	if err != nil || !published.Published {
		t.Fatalf("pending recovery=%+v %v", published, err)
	}
	recovered, err := store.State(ctx, project)
	if err != nil || recovered.Watermark != pending[0].Sequence {
		t.Fatalf("recovery watermark=%+v %v", recovered, err)
	}
	page, err := pluginsdk.Call[rpc.ListInput, rpc.Page](ctx, session, rpc.List, rpc.ListInput{Scope: memory.ScopeProject, CWD: project})
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("recovered private facts=%+v %v", page, err)
	}
	foundHuman, foundProposal := false, false
	for _, item := range page.Rows {
		if item.ID == human.ID {
			foundHuman = item.Content == human.Content && item.Pinned && item.Origin == memory.OriginUser && item.Status == memory.StatusActive
		} else {
			foundProposal = item.Content == "automatic proposal" && item.Status == memory.StatusPending && item.Origin == memory.OriginAuto
		}
	}
	if !foundHuman || !foundProposal {
		t.Fatalf("curation replaced human authority: %+v", page.Rows)
	}
	repeated, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, "recover-pending-ledger", input)
	if err != nil || repeated != published {
		t.Fatalf("settled recovery repeated publication: %+v %v", repeated, err)
	}
	final, err := store.State(ctx, project)
	if err != nil || final != recovered {
		t.Fatalf("settled recovery advanced generation: %+v %v", final, err)
	}
	broker.RevokeSession(session)
}

func TestMaintenancePublicationAndReceiptFailTogetherWithoutLosingLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	project := t.TempDir()
	store, session, _ := connectMemoryBackend(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fail_maintenance_receipt BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failed'); END`); err != nil {
		t.Fatal(err)
	}
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	defer broker.RevokeSession(session)
	var calls atomic.Int32
	grant, release, err := rpc.GrantModels(t.Context(), broker, session, func(context.Context, rpc.Prompt) (string, error) {
		calls.Add(1)
		return "- durable ledger fact", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	input := rpc.MaintenanceInput{SessionID: "ses_receipt_failure", Project: project, Compacted: true, Grant: grant, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured"}}
	if _, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, "original", input); err == nil {
		t.Fatal("receipt failure returned successful publication")
	}
	state, err := store.State(t.Context(), project)
	if err != nil || state.Watermark != 0 {
		t.Fatalf("receipt failure advanced watermark: %+v, %v", state, err)
	}
	pending, err := store.PendingLedger(t.Context(), project, 0, memory.MaxLedgerFoldFacts)
	if err != nil || len(pending) != 1 {
		t.Fatalf("receipt failure discarded committed ledger: %+v, %v", pending, err)
	}
	page, err := pluginsdk.Call[rpc.ListInput, rpc.Page](t.Context(), session, rpc.List, rpc.ListInput{Scope: memory.ScopeProject, CWD: project})
	if err != nil || len(page.Rows) != 0 {
		t.Fatalf("publication survived failed receipt: %+v, %v", page, err)
	}
	if _, found, err := pluginsdk.ReadReceipt[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, "original", rpc.Maintain, input); err != nil || found {
		t.Fatalf("failed publication became known: %t, %v", found, err)
	}
	if _, err := pluginsdk.CallCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, rpc.Maintain, "original", input); err == nil || calls.Load() != 2 {
		t.Fatalf("pending admission reran models: %d, %v", calls.Load(), err)
	}
}
