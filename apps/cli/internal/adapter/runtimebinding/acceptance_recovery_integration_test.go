package runtimebinding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	runapplication "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

type lostOpeningAcknowledgements struct {
	*Connection
	clock    time.Time
	advance  time.Duration
	starts   []prompt.StartRun
	resumes  []conversation.ResumeRun
	queries  []protocol.GetRunAcceptanceRequest
	receipts []protocol.RunAcceptance
}

func (r *lostOpeningAcknowledgements) StartRun(ctx context.Context, command prompt.StartRun) (conversation.SegmentStream, error) {
	r.starts = append(r.starts, command.Clone())
	if _, err := r.Connection.StartRun(ctx, command); err != nil {
		return conversation.SegmentStream{}, err
	}
	r.clock = r.clock.Add(r.advance)
	return conversation.SegmentStream{}, conversation.ErrDisconnected
}

func (r *lostOpeningAcknowledgements) ResumeRun(ctx context.Context, command conversation.ResumeRun) (conversation.SegmentStream, error) {
	r.resumes = append(r.resumes, command.Clone())
	if _, err := r.Connection.ResumeRun(ctx, command); err != nil {
		return conversation.SegmentStream{}, err
	}
	r.clock = r.clock.Add(r.advance)
	return conversation.SegmentStream{}, conversation.ErrDisconnected
}

func (r *lostOpeningAcknowledgements) GetRunAcceptance(ctx context.Context, query protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error) {
	r.queries = append(r.queries, query)
	receipt, err := r.Connection.GetRunAcceptance(ctx, query)
	if err == nil && receipt.Type == protocol.RunAcceptanceAccepted {
		r.receipts = append(r.receipts, receipt)
	}
	return receipt, err
}

func TestOneShotQueriesAcceptedStartAndResumeAfterDeliveryExpiry(t *testing.T) {
	configureIntegrationRuntime(t)
	provider := httptest.NewServer(http.HandlerFunc(runtimefixture.ServeDelegatedApproval))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)
	connection := openIntegrationRuntime(t, runtimefixture.TempDirectory(t))
	if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: runtimefixture.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	profile := connection.Profile()
	limits := profile.Discovery().Capabilities.Limits.Idempotency
	runtime := &lostOpeningAcknowledgements{Connection: connection, clock: time.Now(), advance: time.Duration(limits.RetentionSeconds+1) * time.Second}
	policy, err := mutation.PolicyFromProfile(&profile, func() time.Time { return runtime.clock })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := prompt.StartRun{CommandID: mutation.NewCommandID(), SessionID: session.ID, Message: prompt.Message{Text: "delegate approval probe"}, Options: prompt.RunOptions{Provider: "deepseek", Model: "deepseek-chat"}}
	if err := runapplication.Execute(ctx, runapplication.Invocation{Runtime: runtime, Renderer: new(recoveryRenderer), ReplayPolicy: policy, ApproveAll: true, Start: command}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.starts) != 1 || len(runtime.resumes) != 1 || len(runtime.queries) != 4 || len(runtime.receipts) != 2 {
		t.Fatalf("acceptance recovery redelivered a command: starts=%+v resumes=%+v queries=%+v receipts=%+v", runtime.starts, runtime.resumes, runtime.queries, runtime.receipts)
	}
	if runtime.queries[0] != runtime.queries[1] || runtime.queries[2] != runtime.queries[3] || runtime.queries[0].Key != string(command.CommandID) || runtime.queries[2].Key != string(runtime.resumes[0].CommandID) {
		t.Fatalf("lookup changed original identities: %+v", runtime.queries)
	}
	for _, query := range runtime.queries {
		if query.Namespace != limits.Namespace {
			t.Fatalf("lookup changed original namespace: %+v", query)
		}
	}
	startReceipt, resumeReceipt := runtime.receipts[0], runtime.receipts[1]
	if startReceipt.RunID != resumeReceipt.RunID || startReceipt.SegmentID == resumeReceipt.SegmentID || resumeReceipt.UserItemID != "" {
		t.Fatalf("lookup rewrote original openings: start=%+v resume=%+v", startReceipt, resumeReceipt)
	}
	snapshot, err := connection.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, found := snapshot.LatestRun()
	if !found || root.ID != startReceipt.RunID || root.Status != protocol.RunStatusFinished || root.Outcome.Status != protocol.OutcomeCompleted || len(snapshot.Runs) != 2 || len(snapshot.Interrupts) != 0 {
		t.Fatalf("cold recovery did not complete the original tree: %+v", snapshot)
	}
}
