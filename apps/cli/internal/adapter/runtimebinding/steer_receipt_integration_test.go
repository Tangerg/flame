package runtimebinding

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestSteerReceiptNamesTheUserItemCommittedAtTheNextModelBoundary(t *testing.T) {
	for _, mode := range []protocol.ApprovalMode{protocol.ApprovalModeYolo, protocol.ApprovalModeSafe} {
		t.Run(string(mode), func(t *testing.T) { testSteerReceiptAtModelBoundary(t, mode) })
	}
}

func testSteerReceiptAtModelBoundary(t *testing.T, mode protocol.ApprovalMode) {
	configureIntegrationRuntime(t)
	boundary := newSteerBoundaryProvider()
	defer boundary.release()
	provider := httptest.NewServer(boundary)
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)
	connection := openIntegrationRuntime(t, runtimefixture.TempDirectory(t))
	if _, err := connection.SetApprovalMode(t.Context(), mode); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: runtimefixture.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	diagnostics := &steerDiagnostics{
		t: t, mode: mode, sessionID: session.ID, startedAt: time.Now(),
		run: ctx, boundary: boundary, connection: connection,
	}
	mark, capture := diagnostics.mark, diagnostics.capture
	defer func() {
		if t.Failed() && ctx.Err() == context.DeadlineExceeded {
			inspection, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
			defer stop()
			capture("after deadline", inspection)
		}
	}()
	deadline, _ := ctx.Deadline()
	inspected := make(chan struct{})
	watchdog := time.AfterFunc(time.Until(deadline.Add(-time.Second)), func() {
		defer close(inspected)
		inspection, stop := context.WithDeadline(context.WithoutCancel(t.Context()), deadline)
		defer stop()
		capture("before deadline", inspection)
	})
	defer func() {
		if !watchdog.Stop() {
			<-inspected
		}
	}()
	mark("start requested")
	opened, err := connection.StartRun(ctx, testStartRequest(session.ID, authoring.Message{Text: "steer admission probe"}))
	if err != nil {
		t.Fatal(err)
	}
	mark("start acknowledged; waiting for first provider")
	t.Logf("steer accepted start: session=%s run=%s segment=%s", session.ID, opened.RunID, opened.SegmentID)
	select {
	case <-boundary.firstEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	mark("first provider entered; steer requested")
	receipt, err := connection.SteerRun(ctx, authoring.SteerRun{
		CommandID: "cli_77777777777777777777777777777777",
		RunID:     opened.RunID, SegmentID: opened.SegmentID,
		Message: authoring.Message{Text: "finish with the steered marker"},
	})
	if err != nil || receipt.UserItemID == "" {
		t.Fatalf("accepted steer receipt = %+v, %v", receipt, err)
	}
	mark("steer acknowledged")
	t.Logf("steer accepted input: session=%s run=%s segment=%s item=%s", session.ID, opened.RunID, opened.SegmentID, receipt.UserItemID)
	accepted, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	findReceiptItem := func(block conversation.Block) bool {
		return block.RunID == opened.RunID && block.ID == receipt.UserItemID && block.Kind == conversation.BlockUser
	}
	if slices.ContainsFunc(accepted.Transcript, findReceiptItem) {
		t.Fatal("steer acceptance already claimed application before the model boundary")
	}
	boundary.finishFirst.Do(func() { close(boundary.releaseFirst) })
	if mode == protocol.ApprovalModeSafe {
		mark("first provider released; waiting for approval stream")
		for _, err := range opened.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		waiting, err := connection.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		root, active := waiting.ActiveRun()
		if !active || root.Status != protocol.RunStatusWaiting || len(waiting.Interrupts) != 1 ||
			slices.ContainsFunc(waiting.Transcript, findReceiptItem) {
			t.Fatalf("accepted steer crossed the model boundary before approval: %+v", waiting)
		}
		previousSegment := opened.SegmentID
		mark("waiting snapshot validated; resume requested")
		opened, err = connection.ResumeRun(ctx, conversation.ResumeRun{RunID: opened.RunID, Answers: []conversation.InterruptAnswer{{
			ItemID: conversation.InterruptItemID(waiting.Interrupts[0]), Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove},
		}}})
		if err != nil || opened.SegmentID == previousSegment {
			t.Fatalf("resume after accepted steer: segment=%s err=%v", opened.SegmentID, err)
		}
		mark("resume acknowledged")
		t.Logf("steer accepted resume: session=%s run=%s previous_segment=%s segment=%s", session.ID, opened.RunID, previousSegment, opened.SegmentID)
	}
	mark("waiting for second provider")
	select {
	case <-boundary.secondEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	mark("second provider entered")
	applied, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(applied.Transcript, func(block conversation.Block) bool {
		return findReceiptItem(block) && block.Status == conversation.BlockStatusCompleted && block.Text == "finish with the steered marker"
	}) {
		t.Fatalf("applied receipt Item %s is absent from the authoritative transcript", receipt.UserItemID)
	}
	boundary.finishSecond.Do(func() { close(boundary.releaseSecond) })
	mark("second provider released; waiting for final stream")
	for _, err := range opened.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	finished, err := connection.GetSession(ctx, session.ID)
	if err != nil || !slices.ContainsFunc(finished.Transcript, findReceiptItem) {
		t.Fatalf("finished Run lost its applied steer Item: %v", err)
	}
}

// steerDiagnostics records when each stage of the steer scenario began and
// logs the provider, goroutine and session state if the deadline approaches.
type steerDiagnostics struct {
	t          *testing.T
	mode       protocol.ApprovalMode
	sessionID  string
	startedAt  time.Time
	run        context.Context
	boundary   *steerBoundaryProvider
	connection *Connection
	mu         sync.Mutex
	stages     []steerStage
}

type steerStage struct {
	name    string
	elapsed time.Duration
	at      time.Time
}

func (d *steerDiagnostics) mark(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stages = append(d.stages, steerStage{name, time.Since(d.startedAt), time.Now()})
}

func (d *steerDiagnostics) capture(phase string, inspection context.Context) {
	d.t.Logf("steer diagnostics: phase=%s mode=%s session=%s time=%s elapsed=%s context_error=%v",
		phase, d.mode, d.sessionID, time.Now().UTC().Format(time.RFC3339Nano), time.Since(d.startedAt), d.run.Err())
	d.mu.Lock()
	stages := slices.Clone(d.stages)
	d.mu.Unlock()
	for _, stage := range stages {
		d.t.Logf("steer deadline: mode=%s stage=%s elapsed=%s time=%s", d.mode, stage.name, stage.elapsed, stage.at.UTC().Format(time.RFC3339Nano))
	}
	d.logProvider()
	stack := make([]byte, 1<<20)
	d.t.Logf("steer deadline goroutines:\n%s", stack[:runtime.Stack(stack, true)])
	observed, err := d.connection.GetSession(inspection, d.sessionID)
	d.t.Logf("steer deadline session: snapshot=%+v error=%v", observed, err)
	for _, run := range observed.Runs {
		d.t.Logf("steer deadline run: id=%s segment=%s status=%s outcome=%s explanation=%s",
			run.ID, run.ActiveSegmentID, run.Status, run.Outcome.Status, run.Outcome.Explanation())
	}
}

func (d *steerDiagnostics) logProvider() {
	for name, entered := range map[string]<-chan struct{}{"first": d.boundary.firstEntered, "second": d.boundary.secondEntered} {
		select {
		case <-entered:
			d.t.Logf("steer deadline: %s provider entered", name)
		default:
			d.t.Logf("steer deadline: %s provider has not entered", name)
		}
	}
	for index, request := range d.boundary.recorded() {
		d.t.Logf("steer deadline provider request: index=%d %s", index, request)
	}
}

// steerBoundaryProvider holds the first model call, which proposes the boundary
// tool, until the test has steered, and the second call, which follows that
// tool's result, until the test releases it.
type steerBoundaryProvider struct {
	started                                            time.Time
	firstEntered, secondEntered                        chan struct{}
	releaseFirst, releaseSecond                        chan struct{}
	enterFirst, enterSecond, finishFirst, finishSecond sync.Once
	mu                                                 sync.Mutex
	requests                                           []string
}

type steerBoundaryRequest struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Role       string         `json:"role"`
		Content    jsontext.Value `json:"content"`
		ToolCallID string         `json:"tool_call_id"`
	} `json:"messages"`
	Tools []jsontext.Value `json:"tools"`
}

func newSteerBoundaryProvider() *steerBoundaryProvider {
	return &steerBoundaryProvider{
		started:      time.Now(),
		firstEntered: make(chan struct{}), secondEntered: make(chan struct{}),
		releaseFirst: make(chan struct{}), releaseSecond: make(chan struct{}),
	}
}

func (p *steerBoundaryProvider) release() {
	p.finishFirst.Do(func() { close(p.releaseFirst) })
	p.finishSecond.Do(func() { close(p.releaseSecond) })
}

func (p *steerBoundaryProvider) record(entry string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, entry)
}

func (p *steerBoundaryProvider) recorded() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.requests)
}

func (p *steerBoundaryProvider) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	var body steerBoundaryRequest
	if err := json.UnmarshalRead(request.Body, &body); err != nil {
		p.record(fmt.Sprintf("elapsed=%s decode_error=%v", time.Since(p.started), err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var toolResults []string
	for _, message := range body.Messages {
		if message.Role == "tool" {
			toolResults = append(toolResults, message.ToolCallID)
		}
	}
	p.record(fmt.Sprintf("time=%s elapsed=%s stream=%t tools=%d tool_result_ids=%q",
		time.Now().UTC().Format(time.RFC3339Nano), time.Since(p.started), body.Stream, len(body.Tools), toolResults))
	toolCall := false
	if len(body.Tools) != 0 {
		var held bool
		toolCall, held = p.hold(request.Context(), slices.Contains(toolResults, "steer_boundary"))
		if !held {
			return
		}
	}
	writeSteerBoundaryResponse(w, body.Stream, toolCall)
}

// hold parks one model call at its boundary. It reports whether the call should
// propose the boundary tool and whether the release arrived before the client
// gave up.
func (p *steerBoundaryProvider) hold(ctx context.Context, afterBoundary bool) (toolCall, held bool) {
	wait := p.releaseFirst
	if afterBoundary {
		p.enterSecond.Do(func() { close(p.secondEntered) })
		wait = p.releaseSecond
	} else {
		p.enterFirst.Do(func() { close(p.firstEntered) })
		toolCall = true
	}
	select {
	case <-wait:
		return toolCall, true
	case <-ctx.Done():
		return toolCall, false
	}
}

func writeSteerBoundaryResponse(w http.ResponseWriter, streaming, toolCall bool) {
	delta := map[string]any{"role": "assistant", "content": "steer boundary complete"}
	finish := "stop"
	if toolCall {
		delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
			"index": 0, "id": "steer_boundary", "type": "function",
			"function": map[string]any{"name": "shell", "arguments": `{"command":"printf continued","description":"Continue to the next model boundary"}`},
		}}}
		finish = "tool_calls"
	}
	choice := map[string]any{"index": 0, "finish_reason": finish}
	object := "chat.completion"
	if streaming {
		object = "chat.completion.chunk"
		choice["delta"] = delta
	} else {
		choice["message"] = delta
	}
	body := map[string]any{
		"id": "chatcmpl_steer", "object": object, "created": 1, "model": "deepseek-chat",
		"choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
	}
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		encoded, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, body)
}
