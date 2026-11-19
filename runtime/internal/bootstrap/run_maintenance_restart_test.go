package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/models/catalog"
)

func TestRunMaintenanceUsesCommittedWorkAcrossRestart(t *testing.T) {
	const openingInput = "Read the fixture and ask before finishing."
	const readingInput = "Read the fixture eight times and finish."
	const lightInput = "Finish without using a Tool."
	const questionCallID = "call_question_before_maintenance"
	const limitedModelID = "claude-maintenance-test"
	const limitedModelInput = 14_000
	const memoryFact = "Use the workspace fixture to verify the project."
	const skillName = "verify-after-restart"
	const skillDocument = "---\nname: " + skillName + "\ndescription: Verify the workspace fixture.\n---\nRead fixture.txt after changing the workspace."
	var memoryCalls, miningCalls atomic.Int32
	var openingBatchSent atomic.Bool
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			prompt := request.Messages[0].Text()
			switch {
			case strings.Contains(prompt, "mining a coding-agent conversation for durable facts"), strings.Contains(prompt, "project memory from an immutable fact ledger"):
				memoryCalls.Add(1)
				return completedTextResponse(fmt.Sprintf("[%q]", memoryFact)), nil
			case strings.Contains(prompt, "mining an Agent conversation"):
				miningCalls.Add(1)
				return completedTextResponse(skillDocument), nil
			case strings.HasPrefix(prompt, "You are compacting the earlier portion of a long agent"):
				return completedTextResponse(longContextCompactionSummary), nil
			default:
				return completedTextResponse("Workspace verification"), nil
			}
		}
		for i := len(request.Messages) - 1; i >= 0; i-- {
			message := request.Messages[i]
			if message.Role != chat.RoleUser {
				continue
			}
			switch message.Text() {
			case lightInput:
				return completedTextResponse("Light check finished."), nil
			case readingInput:
				for _, result := range request.Messages[i+1:] {
					for _, part := range result.Parts {
						if part.ToolResult != nil && part.ToolResult.Name == string(builtin.Read) {
							return completedTextResponse("Workspace verification finished."), nil
						}
					}
				}
				return maintenanceReadBatch(""), nil
			}
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.ID == questionCallID {
					return completedTextResponse("Workspace verification finished."), nil
				}
			}
		}
		if requestContainsText(request, longContextCompactionSummary) {
			return toolCallResponse(chat.ToolCall{
				ID: questionCallID, Name: string(builtin.AskUser), Arguments: `{"questions":[{"question":"Finish the verification?"}]}`,
			}), nil
		}
		if !openingBatchSent.CompareAndSwap(false, true) {
			return nil, errors.New("provider received the original Tool batch without compaction")
		}
		return maintenanceReadBatch(strings.Repeat(modelContextPressurePayload, 2)), nil
	})}
	cfg := poMCPConfig(t)
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = staticTestChatResolver{client: &client, metadata: &catalog.Model{
		ID: limitedModelID, ToolCall: true, Limits: catalog.Limits{MaxInputTokens: limitedModelInput},
		Modalities: catalog.Modalities{Input: []catalog.Modality{catalog.ModalityText}, Output: []catalog.Modality{catalog.ModalityText}},
	}}
	cfg.approvalMode = approval.ModeYolo
	if err := os.WriteFile(filepath.Join(cfg.defaultWorkspacePath, "fixture.txt"), []byte("read-only fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := poMCPOpen(t, cfg)
	memory := activateWorkflow(runtime, filepath.Join(firstPartyPackages, "memory"), "restart-memory", "memory", "memory-provider")
	skills := poMCPActivate(runtime, filepath.Join(firstPartyPackages, "skills"))
	memorySource := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: memory.ID}, Name: "memory"}
	runtime.poMCPTools(&memorySource, memoryToolsReady)
	waitCallContributorState(t, runtime, skills.ID, protocol.MCPServerConnected)
	ctx := protocolLifecycleContext(t.Context())
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Run maintenance across restart"}, "maintenance-session").(*protocol.Session)
	started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Provider: cfg.defaultSelection.Provider(), Model: limitedModelID,
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: openingInput}},
	})
	if err != nil {
		t.Fatal(err)
	}
	opened := waitForRunEvents(t, collectRunEvents(events), "Tool batch, compaction and Question")
	if summary, found := compactionSummaryFromEvents(opened); !found || summary != longContextCompactionSummary {
		run := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
		t.Fatalf("compaction summary = %q, found=%t, Run = %s", summary, found, generatedJSON(t, run))
	}
	pending, err := runtime.api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("original Question = %+v, error = %v", pending, err)
	}
	question := pending.Data[0].Interrupts[0]
	runtime = reopenPluginRuntime(t, runtime, &cfg)
	runtime.poMCPTools(&memorySource, memoryToolsReady)
	waitCallContributorState(t, runtime, skills.ID, protocol.MCPServerConnected)
	resumed, events, err := runtime.api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID, Responses: []protocol.InterruptResponse{{
			ItemID: question.ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Yes"}}},
		}},
	})
	if err != nil || resumed.RunID != started.RunID || resumed.SegmentID == started.SegmentID {
		t.Fatalf("resumed = %+v, error = %v", resumed, err)
	}
	waitForRunEvents(t, collectRunEvents(events), "Run maintenance after the answer")
	finished := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("resumed Run = %+v", finished.Outcome)
	}
	memories := readMemoryPage(runtime, memorycontract.ListInput{Scope: memorycontract.ListInputScopeProject, CWD: cfg.defaultWorkspacePath}).Data
	if len(memories) != 1 || memories[0].Content != memoryFact || memoryCalls.Load() != 2 {
		t.Errorf("memory from completed Run = %+v, model requests = %d", memories, memoryCalls.Load())
	}
	if miningCalls.Load() != 0 {
		t.Fatalf("first complex Run triggered mining: %d model requests", miningCalls.Load())
	}
	followUps := []string{readingInput, lightInput, readingInput}
	for index, input := range followUps {
		started, sequence, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
			SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: input}},
		})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(sequence), "subsequent Run")
		finished := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
		if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("subsequent Run = %+v", finished.Outcome)
		}
		if index < len(followUps)-1 && miningCalls.Load() != 0 {
			t.Fatalf("Skills mined before the third qualifying Run: %d model requests", miningCalls.Load())
		}
	}
	target := protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}
	proposals := runtime.must(delivery.SkillsProposalsList, protocol.WorkspaceQuery{Workspace: target}, "").(*protocol.Page[protocol.SkillProposal]).Data
	if len(proposals) != 1 || proposals[0].Name != skillName || proposals[0].SourceSession != session.ID ||
		proposals[0].Origin != protocol.SkillProposalOriginMined || miningCalls.Load() != 1 {
		t.Errorf("skill from three complex Runs = %+v, model requests = %d", proposals, miningCalls.Load())
	}
	if calls := memoryCalls.Load(); calls != 2 {
		t.Errorf("Memory model requests across all Runs = %d, want 2 from the original compaction", calls)
	}
}

func maintenanceReadBatch(text string) *chat.Response {
	const qualifyingCalls = 8
	parts := make([]chat.Part, 0, qualifyingCalls+1)
	if text != "" {
		parts = append(parts, chat.NewTextPart(text))
	}
	for index := range qualifyingCalls {
		parts = append(parts, chat.NewToolCallPart(chat.ToolCall{
			ID: fmt.Sprintf("read-for-maintenance-%d", index), Name: string(builtin.Read), Arguments: `{"path":"fixture.txt"}`,
		}))
	}
	message := chat.NewAssistantMessage(parts...)
	return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}}
}
