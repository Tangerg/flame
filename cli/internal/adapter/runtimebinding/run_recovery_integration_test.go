package runtimebinding

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	runapplication "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunningTreeRecoveryUsesTheRuntimeSnapshotAndSuccessorTail(t *testing.T) {
	configureIntegrationRuntime(t)
	childEntered := make(chan struct{})
	releaseChild := make(chan struct{})
	var entered, release sync.Once
	t.Cleanup(func() { release.Do(func() { close(releaseChild) }) })
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		if bytes.Contains(body, []byte("child approval probe")) {
			entered.Do(func() { close(childEntered) })
			select {
			case <-releaseChild:
			case <-request.Context().Done():
				return
			}
		}
		runtimefixture.ServeDelegatedApproval(w, request)
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)
	connection := openIntegrationRuntime(t, t.TempDir())
	if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeYolo); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	opened, err := connection.StartRun(ctx, prompt.StartRun{
		SessionID: session.ID, Message: prompt.Message{Text: "delegate approval probe"},
		Options: prompt.RunOptions{Provider: "deepseek", Model: "deepseek-chat"},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-childEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	recovered, err := runapplication.RecoverSegment(ctx, connection, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Snapshot.Runs) != 2 || recovered.Stream.Snapshot == nil || recovered.Stream.HeadEventID == "" {
		t.Fatalf("coherent running tree = %+v, stream=%+v", recovered.Snapshot.Runs, recovered.Stream)
	}
	projection := conversation.New()
	if err := projection.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(releaseChild) })
	childFinished := false
	for event, err := range recovered.Stream.Events {
		if err != nil {
			t.Fatal(err)
		}
		if finished, ok := event.Event.(conversation.SegmentFinished); ok && finished.Run.Status == protocol.RunStatusFinished && event.RunID != opened.RunID {
			childFinished = true
		}
		if _, err := projection.ApplyRunEvent(event); err != nil {
			t.Fatalf("apply recovered tree event %s: %v", event.EventID, err)
		}
	}
	if !childFinished || projection.Phase() != conversation.Idle || projection.Outcome().Status != protocol.OutcomeCompleted {
		t.Fatalf("recovered tree: childFinished=%t phase=%s outcome=%+v", childFinished, projection.Phase(), projection.Outcome())
	}
}

func TestOneShotRecoversADelegatedApprovalBeforeResumingTheRoot(t *testing.T) {
	configureIntegrationRuntime(t)
	provider := httptest.NewServer(http.HandlerFunc(runtimefixture.ServeDelegatedApproval))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)

	connection := openIntegrationRuntime(t, t.TempDir())
	if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	profile := connection.Profile()
	replayGuard, err := mutation.PolicyFromProfile(&profile, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &interruptedRootStream{Connection: connection}
	renderer := new(recoveryRenderer)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runapplication.Execute(ctx, runapplication.Invocation{
		Runtime: runtime, Renderer: renderer, ReplayPolicy: replayGuard, ApproveAll: true, Start: prompt.StartRun{
			SessionID: session.ID, Message: prompt.Message{Text: "delegate approval probe"},
			Options: prompt.RunOptions{Provider: "deepseek", Model: "deepseek-chat"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !runtime.disconnected || runtime.subscriptions == 0 || runtime.resumptions != 1 {
		t.Fatalf("recovery: disconnected=%t subscriptions=%d resumptions=%d", runtime.disconnected, runtime.subscriptions, runtime.resumptions)
	}
	snapshot, err := connection.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := snapshot.LatestRun()
	if !ok || root.Status != protocol.RunStatusFinished || root.Outcome.Status != protocol.OutcomeCompleted || len(snapshot.Interrupts) != 0 {
		t.Fatalf("root after recovery = %+v, interrupts = %+v", root, snapshot.Interrupts)
	}
	if len(snapshot.Runs) != 2 {
		t.Fatalf("continuation changed the admitted root and child: %+v", snapshot.Runs)
	}
	if !slices.ContainsFunc(snapshot.Transcript, func(block conversation.Block) bool {
		return block.RunID != root.ID && block.Tool != nil && block.Tool.Name == "shell" &&
			block.Tool.Status == conversation.ToolOK && strings.Contains(block.Tool.Output, "approved") &&
			block.Tool.ExitCode != nil && *block.Tool.ExitCode == 0
	}) {
		t.Fatalf("approved child tool did not complete: %+v", snapshot.Transcript)
	}
	if !slices.ContainsFunc(renderer.events, func(event conversation.RunEvent) bool {
		finished, ok := event.Event.(conversation.SegmentFinished)
		return event.RunID == root.ID && ok && finished.Run.Status == protocol.RunStatusFinished
	}) {
		t.Fatal("the recovered root completion was not rendered")
	}
}

// The durable barrier is committed, but this consumer loses the stream between
// a child's interrupt and the root's suspension event.
type interruptedRootStream struct {
	*Connection
	disconnected  bool
	subscriptions int
	resumptions   int
}

func (r *interruptedRootStream) StartRun(ctx context.Context, request prompt.StartRun) (conversation.SegmentStream, error) {
	stream, err := r.Connection.StartRun(ctx, request)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	events := stream.Events
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		for event, streamErr := range events {
			if finished, ok := event.Event.(conversation.SegmentFinished); ok && finished.Suspended() && event.RunID == stream.RunID {
				r.disconnected = true
				yield(conversation.RunEvent{}, conversation.ErrDisconnected)
				return
			}
			if !yield(event, streamErr) {
				return
			}
		}
	}
	return stream, nil
}

func (r *interruptedRootStream) SubscribeRun(ctx context.Context, request conversation.SubscribeRun) (conversation.SegmentStream, error) {
	r.subscriptions++
	return r.Connection.SubscribeRun(ctx, request)
}

func (r *interruptedRootStream) ResumeRun(ctx context.Context, request conversation.ResumeRun) (conversation.SegmentStream, error) {
	r.resumptions++
	return r.Connection.ResumeRun(ctx, request)
}

type recoveryRenderer struct{ events []conversation.RunEvent }

func (*recoveryRenderer) Begin(conversation.Run, prompt.RunOptions) error { return nil }
func (*recoveryRenderer) Reconcile(conversation.SessionSnapshot) error    { return nil }
func (*recoveryRenderer) Close() error                                    { return nil }
func (r *recoveryRenderer) Render(event conversation.RunEvent) error {
	r.events = append(r.events, event.Clone())
	return nil
}
