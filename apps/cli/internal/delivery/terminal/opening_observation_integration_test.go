package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
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
	}
	host, stop, store := runUIFromConfig(t, Config{
		Runtime: backend, RuntimeProfile: &profile, Workspace: workspace,
		Workspaces: connection, Changes: connection, Transfers: connection, Usage: connection, ModelConfig: connection,
		Goals: connection, Skills: connection, MCP: connection, Plugins: connection,
		DiagnosticTools: connection.DiagnosticTools(), AuthoringContext: connection.AuthoringContext(),
		Hooks: connection.Hooks(), Feedback: connection.Feedback(),
		SessionID: session.ID, OpenWorkbench: func() (*workbench.Store, error) { return workbench.OpenMemory(), nil },
		Settings: settings.Default(),
	})
	release := sync.OnceFunc(func() { close(allowProvider) })
	defer release()
	host.Shows(t, "Ask flame")
	if !host.Type(runtimefixture.DelegateApprovalProbe) || !host.Press(input.Enter) {
		t.Fatal("terminal refused the prompt")
	}
	acceptance, cancelAcceptance := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelAcceptance()
	var accepted protocol.RunAcceptance
	select {
	case accepted = <-backend.accepted:
	case id := <-backend.cancellations:
		t.Fatalf("opening observation canceled the accepted Run %s", id)
	case <-acceptance.Done():
		t.Fatal("terminal did not reconcile the original Run: ", context.Cause(acceptance))
	}
	cancelAcceptance()
	release()
	host.Shows(t, "Tool approval")
	waitForApprovedOpeningRun(t, connection, accepted.RunID, func() {
		if !host.Press(input.Enter) {
			t.Fatal("terminal refused the approval")
		}
	})
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

func waitForApprovedOpeningRun(t *testing.T, connection *runtimebinding.Connection, runID string, approve func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	changes, err := connection.Subscribe(ctx, changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged}})
	if err != nil {
		t.Fatal(err)
	}
	approve()
	for event, err := range changes {
		if err != nil {
			t.Fatalf("observing accepted Run %s: %v", runID, err)
		}
		if event.Type != protocol.RuntimeResync && !slices.Contains(event.RunIDs, runID) {
			continue
		}
		run, err := connection.GetRun(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == protocol.RunStatusFinished {
			if run.Outcome.Status != protocol.OutcomeCompleted {
				t.Fatalf("accepted Run did not complete: %+v", run)
			}
			return
		}
	}
	t.Fatalf("did not observe completion of accepted Run %s: %v", runID, context.Cause(ctx))
}

type openingObservationFault struct {
	*runtimebinding.Connection
	starts        chan authoring.CommandID
	cancellations chan string
	accepted      chan protocol.RunAcceptance
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

func (o *openingObservationFault) CancelRun(ctx context.Context, request conversation.CancelRun) (conversation.RunCancellation, error) {
	o.cancellations <- request.RunID
	return o.Connection.CancelRun(ctx, request)
}
