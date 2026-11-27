package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTerminalRecoversAnAcceptedRunAfterInvalidOpeningObservation(t *testing.T) {
	allowProvider := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-allowProvider:
			runtimefixture.ServeDelegatedApproval(w, request)
		}
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_APIKEY", "integration-key")
	t.Setenv("FLAME_BASEURL", provider.URL)
	workspace := t.TempDir()
	owner := runtimebinding.NewOwner(runtimebinding.Config{
		ProductRoot: runtimefixture.TempDirectory(t), DefaultWorkspacePath: workspace,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	connection, err := owner.Connection(t.Context(), settings.Runtime{Mode: settings.RuntimeEmbedded})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	profile := connection.Profile()
	backend := &openingObservationFault{
		Connection:    connection,
		starts:        make(chan authoring.CommandID, 8),
		cancellations: make(chan string, 8),
		accepted:      make(chan protocol.RunAcceptance, 8),
		finished:      make(chan conversation.Run, 1),
	}
	host, stop, store := runUIFromConfig(t, Config{
		Runtime: backend, RuntimeProfile: &profile, Workspace: workspace,
		Workspaces: connection, Changes: connection, Transfers: connection, Usage: connection, ModelConfig: connection,
		Goals: connection, Skills: connection, MCP: connection, Plugins: connection,
		DiagnosticTools: connection.DiagnosticTools(), AuthoringContext: connection.AuthoringContext(),
		Hooks: connection.Hooks(), Feedback: connection.Feedback(),
		SessionID: session.ID, OpenWorkbench: func() (*workbench.Store, error) { return workbench.OpenMemory(workbench.Config{}) },
		Settings: settings.Default(),
	})
	release := sync.OnceFunc(func() { close(allowProvider) })
	defer release()
	host.Shows(t, "Ask flame")
	if !host.Type(runtimefixture.DelegateApprovalProbe) || !host.Press(input.Enter) {
		t.Fatal("terminal refused the prompt")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var accepted protocol.RunAcceptance
	select {
	case accepted = <-backend.accepted:
	case id := <-backend.cancellations:
		t.Fatalf("opening observation canceled the accepted Run %s", id)
	case <-ctx.Done():
		t.Fatal("terminal neither reconciled nor canceled the original Run: ", context.Cause(ctx))
	}
	release()
	host.Shows(t, "Tool approval")
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused the approval")
	}
	select {
	case finished := <-backend.finished:
		if finished.ID != accepted.RunID || finished.Outcome.Status != protocol.OutcomeCompleted {
			t.Fatalf("observed terminal Run = %+v, receipt = %+v", finished, accepted)
		}
	case <-ctx.Done():
		t.Fatal("accepted Run did not finish: ", context.Cause(ctx))
	}
	host.Shows(t, "root complete")
	select {
	case id := <-backend.cancellations:
		t.Fatalf("the terminal canceled Run %s without an explicit request", id)
	default:
	}
	if starts := len(backend.starts); starts != 1 {
		t.Fatalf("Runtime received %d start deliveries", starts)
	}
	if _, pending := store().Queue().Head(session.ID); pending {
		t.Fatal("confirmed prompt is still pending in the Workbench")
	}
	stop()
	snapshot, err := connection.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, found := snapshot.LatestRun()
	if !found || root.ID != accepted.RunID || root.Outcome.Status != protocol.OutcomeCompleted {
		t.Fatalf("accepted root Run = %+v, receipt = %+v", root, accepted)
	}
}

type openingObservationFault struct {
	*runtimebinding.Connection
	starts        chan authoring.CommandID
	cancellations chan string
	accepted      chan protocol.RunAcceptance
	finished      chan conversation.Run
}

func (o *openingObservationFault) StartRun(ctx context.Context, request authoring.StartRun) (conversation.SegmentStream, error) {
	o.starts <- request.CommandID
	opened, err := o.Connection.StartRun(ctx, request)
	if err == nil {
		opened.SegmentID = ""
	}
	return opened, err
}

func (o *openingObservationFault) GetRunAcceptance(ctx context.Context, request protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error) {
	receipt, err := o.Connection.GetRunAcceptance(ctx, request)
	if err == nil && receipt.Type == protocol.RunAcceptanceAccepted {
		o.accepted <- receipt
	}
	return receipt, err
}

func (o *openingObservationFault) ResumeRun(ctx context.Context, request conversation.ResumeRun) (conversation.SegmentStream, error) {
	stream, err := o.Connection.ResumeRun(ctx, request)
	if err != nil {
		return stream, err
	}
	events := stream.Events
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		for event, err := range events {
			keep := yield(event, err)
			if end, ok := event.Event.(conversation.SegmentFinished); ok && event.RunID == request.RunID && end.Run.Status == protocol.RunStatusFinished {
				o.finished <- end.Run
			}
			if !keep {
				return
			}
		}
	}
	return stream, nil
}

func (o *openingObservationFault) CancelRun(ctx context.Context, request conversation.CancelRun) (conversation.RunCancellation, error) {
	o.cancellations <- request.RunID
	return o.Connection.CancelRun(ctx, request)
}
