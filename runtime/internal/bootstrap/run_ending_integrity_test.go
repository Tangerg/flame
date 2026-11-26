package bootstrap

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestRunReadRefusesMismatchedEndingEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	host, api := openProtocolRuntime(t, newReplyStub("completed"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, stream, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "complete this run"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(stream), "terminal Run")
	query := protocol.GetRunRequest{RunID: started.RunID}
	before, err := api.GetRun(ctx, query)
	if err != nil || before.Outcome == nil || before.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("original Run = %+v, %v", before, err)
	}

	db, err := persistencefixture.OpenRawSQL(home)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "UPDATE runs SET outcome = ?, ending = ? WHERE run_id = ?",
		protocol.OutcomeCanceled, `{"detail":"canceled","loss":{"reason":"restart"}}`, started.RunID)
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}

	result := host.Endpoint().Invoke(ctx, delivery.RunsGet, query, protocol.CallMetadata{
		RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)},
	})
	if result.Value != nil || !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("Run with mismatched durable ending = %+v, %v", result.Value, result.Failure)
	}
}
