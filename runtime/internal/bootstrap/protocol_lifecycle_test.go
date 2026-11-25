package bootstrap

import (
	"bufio"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
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
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestProtocolLifecycleSurvivesColdRestart(t *testing.T) {
	fixture := newProtocolLifecycleFixture(t)
	defer fixture.closeFirstRuntime()
	digest := strings.Repeat("1", 64)
	actor, err := operation.NewActor("00000000-0000-0000-0000-000000000001", digest, digest, "steer")
	if err != nil {
		t.Fatal(err)
	}
	question := fixture.startAndPark(operation.WithActor(fixture.ctx, actor))
	fixture.resumeAndCancel(question)
	fixture.assertColdState()
}

func TestAssemblyPreservesParkedQuestionAcrossCrashLikeRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	parked := crashParkedProtocolRuntime(t)

	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	model := newLifecycleModel()
	model.calls.Store(1)
	cfg := protocolRuntimeConfig(t, stores, model)
	restartedHost, restartedAPI := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	fixture := &protocolLifecycleFixture{
		t: t, home: home, ctx: protocolLifecycleContext(t.Context()), model: model,
		host: restartedHost, api: restartedAPI, stores: stores,
		started: parked.Started, sessionID: parked.SessionID,
	}
	defer fixture.closeFirstRuntime()
	restarted, err := restartedAPI.GetRun(fixture.ctx, protocol.GetRunRequest{RunID: fixture.started.RunID})
	if err != nil {
		t.Fatalf("runs.get after crash-like restart: %v", err)
	}
	pending, err := restartedAPI.ListInterrupts(fixture.ctx, protocol.ListInterruptsRequest{
		RootRunID: fixture.started.RunID,
	})
	if err != nil {
		t.Fatalf("interrupt.list after crash-like restart: %v", err)
	}
	if restarted.Status != protocol.RunStatusWaiting || restarted.Outcome != nil ||
		len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restarted waiting boundary = run %+v, interrupts %+v", restarted, pending.Data)
	}
	fixture.resumeAndCancelWith(restartedAPI, pending.Data[0].Interrupts[0], false)
}

type parkedProtocolMarker struct {
	SessionID string                     `json:"sessionId"`
	Started   *protocol.StartRunResponse `json:"started"`
}

const parkedProtocolPrefix = "FLAME_PROTOCOL_PARKED "

func TestParkedProtocolCrashProcess(t *testing.T) {
	if os.Getenv("FLAME_TEST_PARKED_PROTOCOL_CRASH") != "1" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	model := newLifecycleModel()
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), stores.DataDirectory)
	fixture := &protocolLifecycleFixture{
		t: t, home: home, ctx: protocolLifecycleContext(t.Context()), model: model,
		host: host, api: api, stores: stores,
	}
	t.Cleanup(fixture.closeFirstRuntime)
	fixture.startAndPark(fixture.ctx)
	marker, err := json.Marshal(parkedProtocolMarker{SessionID: fixture.sessionID, Started: fixture.started})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, parkedProtocolPrefix+string(marker)); err != nil {
		t.Fatal(err)
	}
	_, err = bufio.NewReader(os.Stdin).ReadByte()
	t.Fatalf("crash fixture returned before the parent killed it: %v", err)
}

func crashParkedProtocolRuntime(t *testing.T) parkedProtocolMarker {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestParkedProtocolCrashProcess$")
	command.Env = append(os.Environ(), "FLAME_TEST_PARKED_PROTOCOL_CRASH=1")
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	type readiness struct {
		marker parkedProtocolMarker
		err    error
	}
	ready := make(chan readiness, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if value, found := strings.CutPrefix(scanner.Text(), parkedProtocolPrefix); found {
				var marker parkedProtocolMarker
				err := json.Unmarshal([]byte(value), &marker)
				ready <- readiness{marker, err}
				return
			}
		}
		ready <- readiness{err: errors.Join(errors.New("process exited before the durable parked marker"), scanner.Err())}
	}()
	var marker parkedProtocolMarker
	select {
	case value := <-ready:
		if value.err != nil {
			t.Fatal(value.err)
		}
		marker = value.marker
	case <-ctx.Done():
		t.Fatalf("wait for original parked Runtime: %v", ctx.Err())
	}
	if marker.Started == nil || marker.Started.RunID == "" || marker.SessionID == "" {
		t.Fatal("crash process omitted the original run identity")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var killed *exec.ExitError
	err = command.Wait()
	waited = true
	if !errors.As(err, &killed) || killed.ExitCode() != -1 {
		t.Fatalf("original Runtime was not terminated by the operating system: %v", err)
	}
	return marker
}

type protocolLifecycleFixture struct {
	t         *testing.T
	home      string
	ctx       context.Context
	model     *lifecycleModel
	host      *Instance
	api       *delivery.Handler
	stores    *persistence.Bundle
	sessionID string
	started   *protocol.StartRunResponse
	closed    bool
}

func newProtocolLifecycleFixture(t *testing.T) *protocolLifecycleFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := newLifecycleModel()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	host, api := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	ctx := protocolLifecycleContext(t.Context())
	return &protocolLifecycleFixture{
		t: t, home: home, ctx: ctx, model: model, host: host, api: api, stores: stores,
	}
}

func protocolLifecycleContext(ctx context.Context) context.Context {
	return delivery.WithRequestMeta(ctx, protocol.RequestMeta{
		ProtocolVersion: protocol.Version,
		ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	})
}

func (p *protocolLifecycleFixture) startAndPark(steerContext context.Context) protocol.Interrupt {
	p.t.Helper()
	session, err := p.api.CreateSession(p.ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: p.home}, Title: "protocol lifecycle",
	})
	if err != nil {
		p.t.Fatalf("sessions.create: %v", err)
	}
	p.sessionID = session.ID
	started, startEvents, err := p.api.StartRun(p.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Start the lifecycle check.",
		}},
	})
	if err != nil {
		p.t.Fatalf("runs.start: %v", err)
	}
	p.started = started
	if started.RunID == "" || started.SegmentID == "" || started.UserItemID == "" {
		p.t.Fatalf("runs.start returned incomplete identity: %+v", started)
	}
	startEventsDone := collectRunEvents(startEvents)
	select {
	case <-p.model.firstCallStarted:
	case events := <-startEventsDone:
		ended, _ := p.api.GetRun(p.ctx, protocol.GetRunRequest{RunID: started.RunID})
		diagnostic, _ := json.Marshal(ended)
		p.t.Fatalf(
			"run ended before first model call: run=%s domainFailure=%s events=%+v",
			diagnostic,
			p.domainFailureDiagnostic(started.RunID),
			events,
		)
	case <-time.After(lifecycleWaitBudget(p.t)):
		p.t.Fatal("timed out waiting for first model call")
	}

	commandEndpoint := p.host.Endpoint()
	options := acceptedCommandOptions()
	options.IdempotencyKey = "protocol-lifecycle-steer"
	options.RequestMeta.ClientCapabilities.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion}
	query := protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: options.IdempotencyKey, Operation: protocol.RunCommandSteer}
	nomination, err := commandEndpoint.Call[protocol.GetRunAcceptanceRequest, *protocol.RunAcceptance](steerContext, delivery.RunsGetAcceptance, query, protocol.CallMetadata{RequestMeta: options.RequestMeta})
	if err != nil || nomination.Type != protocol.RunAcceptanceNotRecorded || nomination.CandidateUserItemID == "" {
		p.t.Fatalf("steer nomination=%+v,%v", nomination, err)
	}
	steered := commandEndpoint.Invoke(steerContext, delivery.RunsSteer, protocol.SteerRunRequest{
		RunID: started.RunID, ExpectedSegmentID: started.SegmentID,
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: protocolLifecycleSteeringText}},
	}, options)
	if steered.Failure != nil || steered.Value.(*protocol.SteerRunResponse).UserItemID != nomination.CandidateUserItemID {
		p.t.Fatalf("steer acknowledgement=%+v,%v, nomination=%s", steered.Value, steered.Failure, nomination.CandidateUserItemID)
	}
	receipt, err := commandEndpoint.Call[protocol.GetRunAcceptanceRequest, *protocol.RunAcceptance](steerContext, delivery.RunsGetAcceptance, query, protocol.CallMetadata{RequestMeta: options.RequestMeta})
	if err != nil || receipt.Type != protocol.RunAcceptanceAccepted || receipt.UserItemID != nomination.CandidateUserItemID {
		p.t.Fatalf("steer receipt=%+v,%v, nomination=%s", receipt, err, nomination.CandidateUserItemID)
	}
	close(p.model.releaseFirstCall)
	waitForRunEvents(p.t, startEventsDone, "waiting segment")

	waiting, err := p.api.GetRun(p.ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		p.t.Fatalf("runs.get waiting run: %v", err)
	}
	if waiting.Status != protocol.RunStatusWaiting || waiting.Outcome != nil || waiting.ActiveSegmentID != "" {
		p.t.Fatalf("waiting run = %+v, want waiting without outcome or active segment", waiting)
	}
	pending, err := p.api.ListInterrupts(p.ctx, protocol.ListInterruptsRequest{
		RootRunID: started.RunID,
	})
	if err != nil {
		p.t.Fatalf("interrupt.list: %v", err)
	}
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		p.t.Fatalf("pending interrupts = %+v, want one complete set with one interrupt", pending.Data)
	}
	question := pending.Data[0].Interrupts[0]
	if question.RunID != started.RunID || question.Type != protocol.InterruptQuestion {
		p.t.Fatalf("pending interrupt = %+v, want this run's question", question)
	}
	return question
}

func (p *protocolLifecycleFixture) domainFailureDiagnostic(runID string) string {
	if p.stores == nil || p.stores.Runs == nil {
		return "unavailable"
	}
	value, found, err := p.stores.Runs.Run(p.ctx, testsupport.MustRunID(runID))
	if err != nil {
		return "read error: " + err.Error()
	}
	if !found {
		return "missing"
	}
	ending, _ := value.Ending()
	return fmt.Sprintf("%+v", ending)
}

func (p *protocolLifecycleFixture) resumeAndCancel(question protocol.Interrupt) {
	p.resumeAndCancelWith(p.api, question, true)
}

func (p *protocolLifecycleFixture) resumeAndCancelWith(
	api *delivery.Handler,
	question protocol.Interrupt,
	closeFirst bool,
) {
	p.t.Helper()
	resumed, resumeEvents, err := api.ResumeRun(p.ctx, protocol.ResumeRunRequest{
		RunID: p.started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: question.ItemID,
			Response: protocol.InterruptResponseValue{
				Type:    protocol.InterruptResponseAnswer,
				Answers: [][]string{{"Yes"}},
			},
		}},
	})
	if err != nil {
		p.t.Fatalf("runs.resume: %v", err)
	}
	if resumed.RunID != p.started.RunID || resumed.SegmentID == "" || resumed.SegmentID == p.started.SegmentID {
		p.t.Fatalf("runs.resume identity = %+v, want same run and a fresh segment", resumed)
	}
	if resumed.UserItemID != nil {
		p.t.Fatalf("runs.resume userItemId = %q without resume input", *resumed.UserItemID)
	}
	resumeEventsDone := collectRunEvents(resumeEvents)
	select {
	case <-p.model.resumedCallStarted:
	case events := <-resumeEventsDone:
		p.t.Fatalf("resumed segment stopped before its model call: %v; ending: %s", events.err, p.domainFailureDiagnostic(p.started.RunID))
	case <-time.After(lifecycleWaitBudget(p.t)):
		p.t.Fatalf("timed out waiting for resumed model call; ending: %s", p.domainFailureDiagnostic(p.started.RunID))
	}
	items, err := api.ListItems(p.ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: p.started.RunID},
	})
	if err != nil {
		p.t.Fatalf("items.list after accepted answer: %v", err)
	}
	var accepted bool
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeQuestion && item.Question != nil &&
			len(item.Question.Answers) == 1 && len(item.Question.Answers[0]) == 1 &&
			item.Question.Answers[0][0] == "Yes" {
			accepted = true
		}
	}
	if !accepted {
		p.t.Fatalf("items after accepted answer = %+v, want durable Question answers", items.Data)
	}

	type cancelResult struct {
		response *protocol.CancelRunResponse
		err      error
	}
	cancelDone := make(chan cancelResult, 1)
	go func() {
		response, cancelErr := api.CancelRun(p.ctx, protocol.CancelRunRequest{
			RunID: p.started.RunID, Reason: "conformance smoke complete",
		})
		cancelDone <- cancelResult{response: response, err: cancelErr}
	}()
	settlementDelay := time.NewTimer(50 * time.Millisecond)
	<-settlementDelay.C
	close(p.model.releaseResumedCall)
	cancelCall := <-cancelDone
	canceled, err := cancelCall.response, cancelCall.err
	if err != nil {
		p.t.Fatalf("runs.cancel: %v", err)
	}
	if canceled.Type != protocol.CancelRunRoot ||
		canceled.Run.ID != p.started.RunID ||
		canceled.Run.Status != protocol.RunStatusFinished ||
		canceled.Run.Outcome == nil ||
		canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		p.t.Fatalf("runs.cancel result = %+v, want finished(canceled) root", canceled)
	}
	waitForRunEvents(p.t, resumeEventsDone, "canceled segment")
	if closeFirst {
		p.closeFirstRuntime()
	}
}

func (p *protocolLifecycleFixture) assertColdState() {
	p.t.Helper()
	restartedHost, restartedAPI := openProtocolRuntime(p.t, newReplyStub("unused"))
	defer func() {
		if err := restartedHost.Close(); err != nil {
			p.t.Errorf("close restarted runtime: %v", err)
		}
	}()

	recovered, err := restartedAPI.GetRun(p.ctx, protocol.GetRunRequest{RunID: p.started.RunID})
	if err != nil {
		p.t.Fatalf("runs.get after restart: %v", err)
	}
	if recovered.Status != protocol.RunStatusFinished ||
		recovered.Outcome == nil ||
		recovered.Outcome.Type != protocol.OutcomeCanceled ||
		recovered.ActiveSegmentID != "" {
		p.t.Fatalf("cold run = %+v, want finished(canceled) without active segment", recovered)
	}
	items, err := restartedAPI.ListItems(p.ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{
			Type:      protocol.ItemScopeSession,
			SessionID: p.sessionID,
		},
	})
	if err != nil {
		p.t.Fatalf("items.list after restart: %v", err)
	}
	if !hasUserText(items.Data, "Start the lifecycle check.") {
		p.t.Fatal("cold items do not contain the opening user message")
	}
	if !hasUserText(items.Data, protocolLifecycleSteeringText) {
		p.t.Fatal("cold items do not contain the structured steering message")
	}
	snapshot, err := restartedAPI.GetSessionSnapshot(p.ctx, protocol.GetSessionSnapshotRequest{SessionID: p.sessionID})
	if err != nil || len(snapshot.SteerInputs) != 1 || len(snapshot.SteerInputs[0].Input) != 1 || snapshot.SteerInputs[0].Input[0].Text != protocolLifecycleSteeringText {
		p.t.Fatalf("cold Steer origin=%+v,%v", snapshot, err)
	}
	applied := false
	for _, item := range snapshot.Items {
		if item.ID == snapshot.SteerInputs[0].UserItemID && item.Type == protocol.ItemTypeUserMessage && item.RunID == p.started.RunID {
			applied = true
		}
	}
	if !applied {
		p.t.Fatal("cold Steer origin lost its exact committed User Item")
	}
}

func (p *protocolLifecycleFixture) closeFirstRuntime() {
	p.t.Helper()
	if p.closed {
		return
	}
	if err := p.host.Close(); err != nil {
		p.t.Fatalf("close first runtime: %v", err)
	}
	p.closed = true
}

const protocolLifecycleSteeringText = "Keep the resumed answer concise."

func hasUserText(items []protocol.Item, want string) bool {
	for _, item := range items {
		if item.Type != protocol.ItemTypeUserMessage {
			continue
		}
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText && block.Text == want {
				return true
			}
		}
	}
	return false
}
