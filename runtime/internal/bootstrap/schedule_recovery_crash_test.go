//go:build darwin || linux

package bootstrap

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
)

const scheduleCrashKey = "schedule-crash-start"

func scheduleCrashModel(t *testing.T, home string) chat.Model {
	return delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		const name = "schedules_create_schedule"
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == name {
					result, err := json.Marshal(part.ToolResult.Output.Details)
					if err != nil {
						return nil, err
					}
					if err := os.WriteFile(filepath.Join(home, "recovered-model-result.json"), result, 0600); err != nil {
						return nil, err
					}
					completion := completedTextResponse("scheduled from the original result")
					completion.Metadata = &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}}
					return completion, nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:schedules_create_schedule"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call = chat.ToolCall{ID: "original_create", Name: name, Arguments: `{"title":"original title","instructions":"inspect the original progress","cron":"0 0 1 1 *"}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
}

func TestScheduleRecoveryCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_SCHEDULE_CRASH")
	if phase == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	if phase == "resolution" {
		tx := stores.Transactor
		stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
			if err := tx(ctx, apply); err != nil {
				return err
			}
			receipt, err := stores.CommandAcceptances.Lookup(ctx, scheduleCrashKey)
			if err != nil || !receipt.Found {
				return err
			}
			if _, found, err := stores.ExecutorCheckpoints.LoadSessionCheckpoint(ctx, receipt.Acceptance.SessionID); err != nil || !found {
				return err
			}
			evidence := os.Getenv("FLAME_TEST_SCHEDULE_EVIDENCE")
			if err := scheduleCrashReady(evidence); err != nil {
				return err
			}
			permit, err := os.OpenFile(evidence+".continue", os.O_RDONLY, 0600)
			if err != nil {
				return err
			}
			var acknowledgement [1]byte
			_, err = io.ReadFull(permit, acknowledgement[:])
			return errors.Join(err, permit.Close())
		}
	}

	cfg := protocolRuntimeConfig(t, stores, scheduleCrashModel(t, home))
	cfg.approvalMode = approval.ModeYolo
	if os.Getenv("FLAME_TEST_SCHEDULE_EDITED") == "1" {
		cfg.approvalMode = approval.ModeSafe
	}
	executable, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, err = io.Copy(hash, executable)
	if err := errors.Join(err, executable.Close()); err != nil {
		t.Fatal(err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	cfg.buildID = runs.BuildFromSHA256(sum).String()
	host, api := buildProtocolRuntime(t, cfg, home)
	defer host.Close()
	endpoint := acceptedCommandEndpoint(t, api, stores)
	// Mirror production delivery: publish the callback RPC, then run the deferred
	// uncertain-execution revival that depends on background plugin sessions.
	if err := host.application.publishCallbacks(endpoint); err != nil {
		t.Fatal(err)
	}
	if err := host.application.recoverUncertainExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if phase == "commit" || phase == "edited" {
		r := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
		installed := poMCPActivate(r, os.Getenv("FLAME_TEST_SCHEDULE_SOURCE"))
		server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
		r.poMCPTools(server, scheduleToolsReady)
		session, err := api.CreateSession(t.Context(), protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
		if err != nil {
			t.Fatal(err)
		}
		options := acceptedCommandOptions()
		options.IdempotencyKey = scheduleCrashKey
		started := endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Create the recurring original progress inspection."}}}, options)
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		for _, err := range started.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		if phase == "edited" {
			runID := started.Value.(*protocol.StartRunResponse).RunID
			pending, err := api.ListInterrupts(acceptedCrashContext(t), protocol.ListInterruptsRequest{RootRunID: runID})
			if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
				t.Fatalf("approval: %+v,%v", pending, err)
			}
			request := protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove, EditedArgs: map[string]any{"title": "edited title", "instructions": "inspect the original progress", "cron": "0 0 1 1 *"}}}}}
			options := acceptedCommandOptions()
			options.IdempotencyKey = "edited-resume"
			resumed := endpoint.Invoke(t.Context(), delivery.RunsResume, request, options)
			if resumed.Failure != nil {
				t.Fatal(resumed.Failure)
			}
			for _, err := range resumed.Events {
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Fatal("original result was not withheld")
	}
	receipt, err := stores.CommandAcceptances.Lookup(t.Context(), scheduleCrashKey)
	if err != nil || !receipt.Found {
		t.Fatalf("original receipt: %+v,%v", receipt, err)
	}
	segmentID := receipt.Acceptance.SegmentID
	if receipt.CurrentRun != nil && receipt.CurrentRun.ActiveSegmentID() != "" {
		segmentID = receipt.CurrentRun.ActiveSegmentID()
	}
	_, events, err := api.SubscribeRun(acceptedCrashContext(t), protocol.SubscribeRunRequest{RunID: receipt.Acceptance.RunID, SegmentID: segmentID, Snapshot: true})
	if err != nil && !errors.Is(err, protocol.ErrRunFinished) {
		t.Fatal(err)
	}
	if err == nil {
		for _, err := range events {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	value, err := api.GetRun(acceptedCrashContext(t), protocol.GetRunRequest{RunID: receipt.Acceptance.RunID})
	want := protocol.OutcomeCompleted
	if phase == "absent" || phase == "conflict" || phase == "malformed" || phase == "disabled" {
		want = protocol.OutcomeLost
	}
	if err != nil || value.Outcome == nil || value.Outcome.Type != want {
		lookup, _ := os.ReadFile(os.Getenv("FLAME_TEST_SCHEDULE_EVIDENCE") + ".finish")
		t.Fatalf("recovered Run: %+v, outcome=%+v, error=%v, lookup=%s", value, value.Outcome, err, lookup)
	}
}

func scheduleCrashReady(evidence string) error {
	ready, err := os.OpenFile(evidence+".ready", os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = ready.WriteString("ready\n")
	return errors.Join(err, ready.Close())
}
