package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	modernsqlite "modernc.org/sqlite"
)

const (
	rollbackCrashMethod   = delivery.Name("sessions.rollback")
	rollbackCrashKey      = "rollback-acknowledgement-crash"
	rollbackCrashProvider = "openai"
	rollbackCrashModel    = "gpt-5.6-sol"
	rollbackReceiptWrite  = "receipt write"
	rollbackReceiptCommit = "receipt commit"
)

type rollbackCommitCrashStore struct{ operation.ReplayStore }

func (s rollbackCommitCrashStore) Transact(ctx context.Context, write func(context.Context) error) error {
	if err := s.ReplayStore.Transact(ctx, write); err != nil {
		return err
	}
	os.Exit(commandCrashExit)
	return nil
}

func TestRollbackAcknowledgementCrashHelper(t *testing.T) {
	cut := os.Getenv("FLAME_TEST_ROLLBACK_CRASH_CUT")
	if cut == "" {
		return
	}
	if cut == rollbackReceiptWrite {
		if err := modernsqlite.RegisterScalarFunction("test_crash_rollback_ack", 0, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()
	cfg := protocolRuntimeConfig(t, stores, newReplyStub("unused"))
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection(rollbackCrashProvider, rollbackCrashModel)
	host, api := buildProtocolRuntime(t, cfg, home)
	defer host.Close()
	endpoint := host.Endpoint()
	switch cut {
	case rollbackReceiptWrite:
		questionCrashDatabase(t, home, `CREATE TRIGGER rollback_ack_crash BEFORE UPDATE OF payload ON idempotency_records BEGIN SELECT test_crash_rollback_ack(); END`)
	case rollbackReceiptCommit:
		endpoint, err = delivery.NewEndpoint(api, delivery.EndpointConfig{
			Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
			IdempotencyStore: rollbackCommitCrashStore{ReplayStore: stores.Idempotency},
			ValidateStorage:  stores.ValidateDirectory,
		})
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown rollback crash cut %q", cut)
	}
	meta := protocol.CallMetadata{IdempotencyKey: rollbackCrashKey}
	result := endpoint.Invoke(t.Context(), rollbackCrashMethod, protocol.RollbackSessionRequest{
		SessionID:   os.Getenv("FLAME_TEST_ROLLBACK_CRASH_SESSION"),
		ToRunID:     os.Getenv("FLAME_TEST_ROLLBACK_CRASH_KEEP"),
		RestoreType: protocol.RestoreHistory,
	}, meta)
	t.Fatalf("rollback acknowledgement crash not reached: %+v", result.Failure)
}

func TestRollbackAcknowledgementSurvivesProcessCrash(t *testing.T) {
	for _, cut := range []string{rollbackReceiptWrite, rollbackReceiptCommit} {
		t.Run(cut, func(t *testing.T) { testRollbackAcknowledgementCrash(t, cut) })
	}
}

func testRollbackAcknowledgementCrash(t *testing.T, cut string) {
	t.Helper()
	const discardedInput = "Original draft to return after rollback"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/responses/input_tokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"object":"response.input_tokens","input_tokens":16}`)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.output_text.delta","item_id":"message","output_index":0,"content_index":0,"delta":"completed"}`)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf(`{"type":"response.completed","response":{"id":"response","model":%q,"status":"completed","output":[{"type":"message","id":"message","role":"assistant","content":[{"type":"output_text","text":"completed"}]}],"usage":{"input_tokens":7,"output_tokens":3}}}`, rollbackCrashModel))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := poMCPConfig(t)
	cfg.maintenance = noMaintenance{}
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection(rollbackCrashProvider, rollbackCrashModel)
	runtime := poMCPOpen(t, cfg)
	provider := activateModelProviders(runtime)
	runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
		Provider: rollbackCrashProvider,
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new("test-key")},
		BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &upstream.URL},
	}, "configure-rollback-provider")
	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Rollback receipt"})
	if err != nil {
		t.Fatal(err)
	}
	var runIDs []string
	for _, input := range []string{"First turn to retain", discardedInput} {
		started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: input}},
		})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), "original turn to complete")
		waitForProtocolRunTerminal(t, ctx, runtime.api, started.RunID)
		runIDs = append(runIDs, started.RunID)
	}
	runtime.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{
		InstallationID: provider.ID,
		Enabled:        false,
	}, "retire-fixture-provider")
	home := cfg.stores.DataDirectory
	runtime.close()
	if err := cfg.stores.Close(); err != nil {
		t.Fatal(err)
	}
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestRollbackAcknowledgementCrashHelper$")
	command.Env = append(os.Environ(), "FLAME_HOME="+home, "FLAME_TEST_ROLLBACK_CRASH_CUT="+cut,
		"FLAME_TEST_ROLLBACK_CRASH_SESSION="+session.ID, "FLAME_TEST_ROLLBACK_CRASH_KEEP="+runIDs[0])
	output, err := command.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("rollback process did not reach the receipt cut: %v\n%s", err, output)
	}
	wantRuns := 1
	if cut == rollbackReceiptWrite {
		questionCrashDatabase(t, home, "DROP TRIGGER rollback_ack_crash")
		wantRuns = 2
	}
	reopenPluginStores(t, &cfg)
	restarted := poMCPOpen(t, cfg)
	beforeReplay, err := restarted.api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(beforeReplay.Data) != wantRuns {
		t.Fatalf("history after %s = %+v, %v, want %d Runs", cut, beforeReplay, err, wantRuns)
	}
	request := protocol.RollbackSessionRequest{SessionID: session.ID, ToRunID: runIDs[0], RestoreType: protocol.RestoreHistory}
	meta := protocol.CallMetadata{IdempotencyKey: rollbackCrashKey}
	for range 2 {
		receipt, err := restarted.endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request, meta)
		if err != nil || receipt == nil || receipt.Session == nil || receipt.Session.ID != session.ID || len(receipt.DroppedRuns) != 1 {
			t.Fatalf("rollback could not recover its original receipt: %+v, %v", receipt, err)
		}
		dropped := receipt.DroppedRuns[0]
		if dropped.Run.ID != runIDs[1] || len(dropped.UserInput) != 1 || dropped.UserInput[0].Text != discardedInput {
			t.Fatalf("rollback receipt changed the dropped original input: %+v", dropped)
		}
	}
	changed := request
	changed.ToRunID = ""
	if response, err := restarted.endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, changed, meta); response != nil || !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed rollback reused the committed key: %+v, %v", response, err)
	}
	remaining, err := restarted.api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(remaining.Data) != 1 || remaining.Data[0].ID != runIDs[0] {
		t.Fatalf("replayed rollback changed the retained timeline: %+v, %v", remaining, err)
	}
}
