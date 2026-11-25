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
	"github.com/Tangerg/flame/runtime/internal/testsupport"
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
	script := &maintenanceScript{
		lightInput: lightInput, readingInput: readingInput, questionCallID: questionCallID,
		memoryFact: memoryFact, skillDocument: skillDocument,
	}
	model := delegateRestartModel{chat.ModelFunc(script.Call)}
	cfg := poMCPConfig(t)
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = staticTestChatResolver{client: &client, metadata: &catalog.Model{
		ID: limitedModelID, ToolCall: true, Limits: catalog.Limits{MaxInputTokens: limitedModelInput},
		Modalities: catalog.Modalities{Input: []catalog.Modality{catalog.ModalityText}, Output: []catalog.Modality{catalog.ModalityText}},
	}, pricing: testsupport.Unpriced()}
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
	memories := readMemoryPage(runtime, memorycontract.ListInput{Scope: memorycontract.ListInputScopeProject, Cwd: cfg.defaultWorkspacePath}).Data
	if len(memories) != 1 || memories[0].Content != memoryFact || script.memoryCalls.Load() != 2 {
		t.Errorf("memory from completed Run = %+v, model requests = %d", memories, script.memoryCalls.Load())
	}
	if script.miningCalls.Load() != 0 {
		t.Fatalf("first complex Run triggered mining: %d model requests", script.miningCalls.Load())
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
		if index < len(followUps)-1 && script.miningCalls.Load() != 0 {
			t.Fatalf("Skills mined before the third qualifying Run: %d model requests", script.miningCalls.Load())
		}
	}
	target := protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}
	proposals := runtime.must(delivery.SkillsProposalsList, protocol.WorkspaceQuery{Workspace: target}, "").(*protocol.Page[protocol.SkillProposal]).Data
	if len(proposals) != 1 || proposals[0].Name != skillName || proposals[0].SourceSession != session.ID ||
		proposals[0].Origin != protocol.SkillProposalOriginMined || script.miningCalls.Load() != 1 {
		t.Errorf("skill from three complex Runs = %+v, model requests = %d", proposals, script.miningCalls.Load())
	}
	if calls := script.memoryCalls.Load(); calls != 2 {
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

// maintenanceScript answers the run maintenance scenario: auxiliary prompts get
// memory facts, a skill or a compaction summary; the opening Run reads the
// fixture under context pressure, asks after compaction and finishes once
// answered; later Runs finish after their own reads.
type maintenanceScript struct {
	lightInput, readingInput, questionCallID string
	memoryFact, skillDocument                string
	memoryCalls, miningCalls                 atomic.Int32
	openingBatchSent                         atomic.Bool
}

func (s *maintenanceScript) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Tools) == 0 {
		return s.auxiliary(request.Messages[0].Text()), nil
	}
	if response, ok := s.latestInputReply(request); ok {
		return response, nil
	}
	if s.questionAnswered(request) {
		return completedTextResponse("Workspace verification finished."), nil
	}
	if requestContainsText(request, longContextCompactionSummary) {
		return toolCallResponse(chat.ToolCall{
			ID: s.questionCallID, Name: string(builtin.AskUser), Arguments: `{"questions":[{"question":"Finish the verification?"}]}`,
		}), nil
	}
	if !s.openingBatchSent.CompareAndSwap(false, true) {
		return nil, errors.New("provider received the original Tool batch without compaction")
	}
	return maintenanceReadBatch(strings.Repeat(modelContextPressurePayload, 2)), nil
}

func (s *maintenanceScript) auxiliary(prompt string) *chat.Response {
	switch {
	case strings.Contains(prompt, "mining a coding-agent conversation for durable facts"), strings.Contains(prompt, "project memory from an immutable fact ledger"):
		s.memoryCalls.Add(1)
		return completedTextResponse(fmt.Sprintf("[%q]", s.memoryFact))
	case strings.Contains(prompt, "mining an Agent conversation"):
		s.miningCalls.Add(1)
		return completedTextResponse(s.skillDocument)
	case strings.HasPrefix(prompt, "You are compacting the earlier portion of a long agent"):
		return completedTextResponse(longContextCompactionSummary)
	default:
		return completedTextResponse("Workspace verification")
	}
}

// latestInputReply answers the light and reading Runs by their latest user input.
func (s *maintenanceScript) latestInputReply(request *chat.Request) (*chat.Response, bool) {
	for i := len(request.Messages) - 1; i >= 0; i-- {
		message := request.Messages[i]
		if message.Role != chat.RoleUser {
			continue
		}
		switch message.Text() {
		case s.lightInput:
			return completedTextResponse("Light check finished."), true
		case s.readingInput:
			if hasReadResult(request.Messages[i+1:]) {
				return completedTextResponse("Workspace verification finished."), true
			}
			return maintenanceReadBatch(""), true
		}
	}
	return nil, false
}

func hasReadResult(messages []chat.Message) bool {
	for _, result := range messages {
		for _, part := range result.Parts {
			if part.ToolResult != nil && part.ToolResult.Name == string(builtin.Read) {
				return true
			}
		}
	}
	return false
}

func (s *maintenanceScript) questionAnswered(request *chat.Request) bool {
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.ToolResult != nil && part.ToolResult.ID == s.questionCallID {
				return true
			}
		}
	}
	return false
}
