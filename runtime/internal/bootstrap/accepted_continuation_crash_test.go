package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	runapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/localruntime"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

func acceptedCrashModel(t *testing.T, home string) chat.Model {
	t.Helper()
	return delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			if message.Role == chat.RoleTool {
				matches := 0
				for _, candidate := range request.Messages {
					if candidate.Role == chat.RoleUser && candidate.Text() == "retain this accepted input" {
						matches++
					}
				}
				if matches != 1 {
					return nil, errors.New("accepted continuation lost its additional input")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "approved_shell", Name: "shell", Arguments: `{"command":"printf executed >> effect.txt","description":"Write the effect"}`}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
}

func TestAcceptedContinuationCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_ACCEPTED_CRASH")
	if phase == "" {
		return
	}
	if phase == "release" {
		if err := modernsqlite.RegisterScalarFunction("test_accepted_release_crash", 0, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
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
	var waitingHead runapp.ExecutionTreeHead
	installCrash := func() {
		tx := stores.Transactor
		stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
			rollback := errors.New("test acceptance rollback")
			err := tx(ctx, func(txCtx context.Context) error {
				if err := apply(txCtx); err != nil {
					return err
				}
				if phase == "rollback" {
					bindings, err := stores.AcceptedContinuations.List(txCtx)
					if err != nil {
						return err
					}
					if len(bindings) > 0 {
						return rollback
					}
				}
				return nil
			})
			if errors.Is(err, rollback) {
				os.Exit(commandCrashExit)
			}
			if err != nil {
				return err
			}
			bindings, err := stores.AcceptedContinuations.List(ctx)
			if err != nil {
				return err
			}
			if len(bindings) > 0 && (phase == "acceptance" || phase == "cold_acceptance") {
				if phase == "cold_acceptance" {
					current, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(ctx, waitingHead.SessionID, waitingHead.RootID)
					if err != nil || !found || !waitingHead.SameCommit(current) {
						return fmt.Errorf("writer changed before acceptance: found=%t, error=%v", found, err)
					}
				}
				os.Exit(commandCrashExit)
			}
			return nil
		}
	}
	installCrash()
	cfg := protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
	cfg.ApprovalMode = approval.ModeSafe
	host, api := buildProtocolRuntime(t, cfg, home)
	defer host.Close()
	ctx := acceptedCrashContext(t)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "accepted crash"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := acceptedCommandEndpoint(t, api, stores)
	startOptions := acceptedCommandOptions()
	startOptions.IdempotencyKey = "start-crash-fixture"
	startResult := endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "write after approval"}}}, startOptions)
	if startResult.Failure != nil {
		t.Fatal(startResult.Failure)
	}
	started := startResult.Value.(*protocol.StartRunResponse)
	for _, err := range startResult.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("pending=%+v, %v", pending, err)
	}
	if phase == "cold_acceptance" {
		if err := host.Close(); err != nil {
			t.Fatal(err)
		}
		stores, err = persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
		if err != nil {
			t.Fatal(err)
		}
		installCrash()
		cfg = protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
		cfg.ApprovalMode = approval.ModeSafe
		host, api = buildProtocolRuntime(t, cfg, home)
		defer host.Close()
		endpoint = acceptedCommandEndpoint(t, api, stores)
		stored, found, err := stores.Interrupts.Get(ctx, started.RunID)
		if err != nil || !found {
			t.Fatalf("cold waiting handoff=%t,%v", found, err)
		}
		member, _ := stored.RootContinuation()
		waitingHead, found, err = stores.ExecutorCheckpoints.LoadExecutionTree(ctx, session.ID, member.MemberID)
		if err != nil || !found {
			t.Fatalf("cold waiting head=%t,%v", found, err)
		}
	}
	if phase == "release" {
		directory, err := localruntime.DataDirectoryAt(home)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", directory.DatabasePath())
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(t.Context(), `CREATE TRIGGER accepted_release_crash BEFORE UPDATE ON execution_trees WHEN instr(CAST(OLD.payload AS TEXT), 'answer:') > 0 BEGIN SELECT test_accepted_release_crash(); END`)
		if err := errors.Join(err, db.Close()); err != nil {
			t.Fatal(err)
		}
	}
	result := endpoint.Invoke(ctx, delivery.RunsResume, acceptedResumeRequest(started.RunID, pending.Data[0].Interrupts[0].ItemID), acceptedCommandOptions())
	if phase == "release" && result.Failure == nil {
		for _, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Fatalf("accepted crash was not reached: %v", result.Failure)
}

const acceptedCommandKey = "resume-crash"

func acceptedCommandOptions() delivery.Options {
	return delivery.Options{IdempotencyKey: acceptedCommandKey, IdempotencyNamespace: testsupport.IdempotencyNamespace,
		RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}}}}
}

func acceptedCommandEndpoint(t *testing.T, api *delivery.Handler, stores *persistence.Bundle) *delivery.Endpoint {
	t.Helper()
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: stores.Idempotency, IdempotencyNamespace: testsupport.IdempotencyNamespace})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func acceptedResumeRequest(runID, itemID string) protocol.ResumeRunRequest {
	return protocol.ResumeRunRequest{RunID: runID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "retain this accepted input"}},
		Responses: []protocol.InterruptResponse{{ItemID: itemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove, Remember: &protocol.RememberScope{Scope: protocol.RememberGlobal}}}}}
}

func acceptedCrashContext(t *testing.T) context.Context {
	return delivery.WithRequestMeta(t.Context(), acceptedCommandOptions().RequestMeta)
}

func TestAcceptedContinuationSurvivesAbruptRestart(t *testing.T) {
	for _, phase := range []string{"acceptance", "cold_acceptance", "release"} {
		t.Run(phase, func(t *testing.T) { testAcceptedContinuationSurvivesRestart(t, phase) })
	}
}

func testAcceptedContinuationSurvivesRestart(t *testing.T, phase string) {
	home, stores := acceptedCrashStores(t, phase)
	bindings, err := stores.AcceptedContinuations.List(t.Context())
	if err != nil || len(bindings) != 1 {
		t.Fatalf("accepted bindings=%+v, %v", bindings, err)
	}
	accepted := bindings[0]
	record, err := stores.CommandAcceptances.Lookup(t.Context(), acceptedCommandKey)
	if err != nil || !record.Found || record.Acceptance.Operation != "resume" || record.Acceptance.SegmentID != accepted.Segments[len(accepted.Segments)-1].SegmentID || record.Acceptance.UserItemID != accepted.InputItemID {
		t.Fatalf("crash lost original command acceptance: %+v,%v", record, err)
	}
	if _, err := os.Stat(filepath.Join(home, "effect.txt")); !os.IsNotExist(err) {
		t.Fatalf("unactivated Tool executed: %v", err)
	}
	// Forgetting a rule after acceptance must survive revival: only the original
	// acceptance transaction may create it.
	rules, err := stores.ApprovalRules.Visible(t.Context(), accepted.SessionID, home, approval.MaximumVisibleRules+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("accepted rules=%+v", rules)
	}
	if err := stores.ApprovalRules.Delete(t.Context(), rules[0].ID); err != nil {
		t.Fatal(err)
	}
	cfg := protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
	cfg.ApprovalMode = approval.ModeSafe
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	waitForProtocolRunTerminal(t, acceptedCrashContext(t), api, accepted.RootRunID())
	endpoint := acceptedCommandEndpoint(t, api, stores)
	lookup := protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: acceptedCommandKey, Operation: protocol.RunCommandResume}
	ack, err := endpoint.Call[protocol.GetRunAcceptanceRequest, *protocol.RunAcceptance](t.Context(), delivery.RunsGetAcceptance, lookup, delivery.Options{})
	if err != nil || ack.SegmentID != record.Acceptance.SegmentID || ack.UserItemID != accepted.InputItemID || ack.Run == nil || ack.Run.Status != protocol.RunStatusFinished {
		t.Fatalf("acceptance lookup after terminal=%+v,%v", ack, err)
	}
	request := acceptedResumeRequest(accepted.RootRunID(), accepted.Pending.Bindings[0].InterruptItemID)
	redelivered := endpoint.Invoke(t.Context(), delivery.RunsResume, request, acceptedCommandOptions())
	if redelivered.Failure != nil {
		t.Fatalf("terminal Resume redelivery=%v", redelivered.Failure)
	}
	response := redelivered.Value.(*protocol.ResumeRunResponse)
	if response.SegmentID != ack.SegmentID || response.UserItemID == nil || *response.UserItemID != ack.UserItemID {
		t.Fatalf("redelivery changed acceptance=%+v", response)
	}
	for event, err := range redelivered.Events {
		t.Fatalf("terminal redelivery produced work=%+v,%v", event, err)
	}
	changed := request
	changed.Input = []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "different input"}}
	if failure := endpoint.Invoke(t.Context(), delivery.RunsResume, changed, acceptedCommandOptions()).Failure; !errors.Is(failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed Resume accepted=%v", failure)
	}
	value, err := api.GetRun(t.Context(), protocol.GetRunRequest{RunID: accepted.RootRunID()})
	if err != nil || value.Outcome == nil || value.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("revived run=%+v, outcome=%+v, %v", value, value.Outcome, err)
	}

	content, err := os.ReadFile(filepath.Join(home, "effect.txt"))
	if err != nil || string(content) != "executed" {
		t.Fatalf("effect=%q, %v", content, err)
	}
	rules, err = stores.ApprovalRules.Visible(t.Context(), accepted.SessionID, home, approval.MaximumVisibleRules+1)
	if err != nil || len(rules) != 0 {
		t.Fatalf("revival recreated forgotten rule: %+v, %v", rules, err)
	}
	remaining, err := stores.AcceptedContinuations.List(t.Context())
	if err != nil || len(remaining) != 0 {
		t.Fatalf("delivered binding retained: %+v, %v", remaining, err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	cold, api := openProtocolRuntime(t, acceptedCrashModel(t, home))
	t.Cleanup(func() { _ = cold.Close() })
	after, err := api.GetRun(t.Context(), protocol.GetRunRequest{RunID: accepted.RootRunID()})
	if err != nil || !reflect.DeepEqual(after, value) {
		t.Fatalf("terminal changed: %+v, %v", after, err)
	}
	content, _ = os.ReadFile(filepath.Join(home, "effect.txt"))
	if strings.Count(string(content), "executed") != 1 {
		t.Fatalf("terminal restart repeated effect: %q", content)
	}
}

func acceptedCrashStores(t *testing.T, phase string) (string, *persistence.Bundle) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcceptedContinuationCrashHelper$")
	command.Env = append(os.Environ(), "FLAME_TEST_ACCEPTED_CRASH="+phase)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
		t.Fatalf("crash: %v\n%s", err, output)
	}
	if phase == "release" {
		directory, err := localruntime.DataDirectoryAt(home)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", directory.DatabasePath())
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(t.Context(), `DROP TRIGGER accepted_release_crash`)
		if err := errors.Join(err, db.Close()); err != nil {
			t.Fatal(err)
		}
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	return home, stores
}

func TestAcceptedRevivalRejectsUnsafeBoundaries(t *testing.T) {
	for _, scenario := range []string{"corrupt context", "corrupt head", "other build", "cancellation won", "completed"} {
		t.Run(scenario, func(t *testing.T) {
			home, stores := acceptedCrashStores(t, "acceptance")
			bindings, err := stores.AcceptedContinuations.List(t.Context())
			if err != nil || len(bindings) != 1 {
				t.Fatalf("bindings=%+v,%v", bindings, err)
			}
			accepted := bindings[0]
			cfg := protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
			cfg.ApprovalMode = approval.ModeSafe
			expected := protocol.OutcomeLost
			switch scenario {
			case "corrupt context":
				member, _ := accepted.Pending.RootContinuation()
				checkpoint, err := stores.ExecutorCheckpoints.LoadCheckpoint(t.Context(), member.MemberID)
				if err != nil {
					t.Fatal(err)
				}
				checkpoint.Payload = []byte(`{"unexpected":true}`)
				if err := stores.ExecutorCheckpoints.SaveCheckpoint(t.Context(), checkpoint); err != nil {
					t.Fatal(err)
				}
			case "corrupt head":
				directory, err := localruntime.DataDirectoryAt(home)
				if err != nil {
					t.Fatal(err)
				}
				db, err := sqlite.Open(t.Context(), directory.DatabasePath())
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.ExecContext(t.Context(), `UPDATE execution_trees SET payload = '{}'`)
				if err := errors.Join(err, db.Close()); err != nil {
					t.Fatal(err)
				}
			case "other build":
				cfg.BuildID = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
			case "cancellation won", "completed":
				value, found, err := stores.Runs.Run(t.Context(), accepted.RootRunID())
				if err != nil || !found {
					t.Fatalf("Run=%+v,%v", value, err)
				}
				outcome := run.OutcomeCanceled
				expected = protocol.OutcomeCanceled
				if scenario == "completed" {
					outcome = run.OutcomeCompleted
					expected = protocol.OutcomeCompleted
				}
				replacement, err := run.Replace(value, func(value run.Run) (run.Run, error) {
					return value.Terminate(run.Termination{Outcome: outcome, FinishedAt: time.Now().UTC()})
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := stores.Runs.Terminalize(t.Context(), replacement); err != nil {
					t.Fatal(err)
				}
			}
			host, api := buildProtocolRuntime(t, cfg, home)
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			value, err := api.GetRun(t.Context(), protocol.GetRunRequest{RunID: accepted.RootRunID()})
			if err != nil || value.Outcome == nil || value.Outcome.Type != expected {
				t.Fatalf("Run=%+v,outcome=%+v,%v", value, value.Outcome, err)
			}
			if _, err := os.Stat(filepath.Join(home, "effect.txt")); !os.IsNotExist(err) {
				t.Fatalf("unsafe recovery executed a Tool: %v", err)
			}
			remaining, err := stores.AcceptedContinuations.List(t.Context())
			if err != nil || len(remaining) != 0 {
				t.Fatalf("binding not retired=%+v,%v", remaining, err)
			}
			pending, err := api.ListInterrupts(t.Context(), protocol.ListInterruptsRequest{RootRunID: accepted.RootRunID()})
			if err != nil || len(pending.Data) != 0 {
				t.Fatalf("accepted input became open again: %+v,%v", pending, err)
			}
		})
	}
}

func TestResumeAcceptanceRollbackSurvivesAbruptRestart(t *testing.T) {
	home, stores := acceptedCrashStores(t, "rollback")
	bindings, err := stores.AcceptedContinuations.List(t.Context())
	if err != nil || len(bindings) != 0 {
		t.Fatalf("rollback kept acceptance: %+v,%v", bindings, err)
	}
	if record, err := stores.CommandAcceptances.Lookup(t.Context(), acceptedCommandKey); err != nil || record.Found {
		t.Fatalf("rollback kept command acceptance: %+v,%v", record, err)
	}
	cfg := protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
	cfg.ApprovalMode = approval.ModeSafe
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() { _ = host.Close() })
	pending, err := api.ListInterrupts(acceptedCrashContext(t), protocol.ListInterruptsRequest{})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("rolled back prompt=%+v,%v", pending, err)
	}
	rules, err := api.ListApprovalRules(t.Context(), protocol.ListApprovalRulesRequest{})
	if err != nil || len(rules.Rules) != 0 {
		t.Fatalf("rollback kept rule=%+v,%v", rules, err)
	}
	if _, err := os.Stat(filepath.Join(home, "effect.txt")); !os.IsNotExist(err) {
		t.Fatalf("rollback executed Tool: %v", err)
	}
}

func TestColdResumeWriterFailurePreservesCommandAcceptance(t *testing.T) {
	home, stores := acceptedCrashStores(t, "rollback")
	pending, err := stores.Interrupts.List(t.Context(), "")
	if err != nil || len(pending) != 1 {
		t.Fatalf("waiting fixture=%v,%v", pending, err)
	}
	cfg := protocolRuntimeConfig(t, stores, acceptedCrashModel(t, home))
	cfg.ApprovalMode = approval.ModeSafe
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() { _ = host.Close() })
	directory, err := localruntime.DataDirectoryAt(home)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `CREATE TRIGGER reject_resume_writer BEFORE UPDATE ON execution_trees WHEN NEW.writer <> OLD.writer BEGIN SELECT RAISE(ABORT, 'test writer restoration failed'); END`)
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	endpoint := acceptedCommandEndpoint(t, api, stores)
	request := acceptedResumeRequest(pending[0].RootRunID, pending[0].Bindings[0].InterruptItemID)
	accepted := endpoint.Invoke(t.Context(), delivery.RunsResume, request, acceptedCommandOptions())
	if accepted.Failure != nil {
		t.Fatalf("post-acceptance writer failure rejected the command: %v", accepted.Failure)
	}
	response := accepted.Value.(*protocol.ResumeRunResponse)
	if response.UserItemID == nil {
		t.Fatal("accepted Resume lost its additional input reference")
	}
	for _, err := range accepted.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	value, err := api.GetRun(t.Context(), protocol.GetRunRequest{RunID: response.RunID})
	if err != nil || value.Outcome == nil || value.Outcome.Type != protocol.OutcomeFailed {
		t.Fatalf("writer failure did not terminalize the accepted Run: %+v,%v", value, err)
	}
	record, err := stores.CommandAcceptances.Lookup(t.Context(), acceptedCommandKey)
	if err != nil || !record.Found || record.Acceptance.SegmentID != response.SegmentID || record.Acceptance.UserItemID != *response.UserItemID {
		t.Fatalf("writer failure lost command acceptance: %+v,%v", record, err)
	}
	replayed := endpoint.Invoke(t.Context(), delivery.RunsResume, request, acceptedCommandOptions())
	if replayed.Failure != nil || !reflect.DeepEqual(response, replayed.Value) {
		t.Fatalf("same-key retry changed acceptance: %+v,%v", replayed.Value, replayed.Failure)
	}
	for event, err := range replayed.Events {
		t.Fatalf("failed accepted command restarted work: %+v,%v", event, err)
	}
	if _, err := os.Stat(filepath.Join(home, "effect.txt")); !os.IsNotExist(err) {
		t.Fatalf("writer failure executed the Tool: %v", err)
	}
	if values, err := stores.AcceptedContinuations.List(t.Context()); err != nil || len(values) != 0 {
		t.Fatalf("terminal retained an activation handoff: %+v,%v", values, err)
	}
}
