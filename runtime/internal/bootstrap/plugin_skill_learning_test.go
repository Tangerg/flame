package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestCompletedRunsUseSelectedSkillLearningAndHumanPublication(t *testing.T) {
	var miningCalls atomic.Int32
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			if strings.Contains(request.Messages[0].Text(), "mining an Agent conversation") {
				miningCalls.Add(1)
				return completedTextResponse("---\nname: verify-workspace\ndescription: Verify Go changes in this workspace.\n---\nRun go test ./... after changing Go code."), nil
			}
			return completedTextResponse("Workspace checked"), nil
		}
		lastUser := 0
		for i, message := range request.Messages {
			if message.Role == chat.RoleUser && message.Text() == "Read the fixture eight times and finish." {
				lastUser = i
			}
		}
		for _, message := range request.Messages[lastUser:] {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == "read" {
					return completedTextResponse("Read complete"), nil
				}
			}
		}
		parts := make([]chat.Part, 8)
		for i := range parts {
			parts[i] = chat.NewToolCallPart(chat.ToolCall{ID: fmt.Sprintf("read-%d", i), Name: "read", Arguments: `{"path":"fixture.txt"}`})
		}
		message := chat.NewAssistantMessage(parts...)
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	cfg.approvalMode = approval.ModeYolo
	if err := os.WriteFile(filepath.Join(cfg.defaultWorkspacePath, "fixture.txt"), []byte("read-only fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "skills"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Learning lifecycle"}, "session").(*protocol.Session)
	target := protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}
	run := func(index int) {
		t.Helper()
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Read the fixture eight times and finish."}}}, delivery.Options{IdempotencyKey: fmt.Sprintf("learning-run-%d", index), RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		for _, err := range started.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		completed := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
		if completed.Outcome == nil || completed.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("run did not complete: %+v", completed)
		}
	}
	for i := range 2 {
		run(i)
	}
	if proposals := r.must(delivery.SkillsProposalsList, protocol.WorkspaceQuery{Workspace: target}, "").(*protocol.Page[protocol.SkillProposal]).Data; len(proposals) != 0 || miningCalls.Load() != 0 {
		t.Fatal("learning cadence was bypassed")
	}
	run(2)
	proposals := r.must(delivery.SkillsProposalsList, protocol.WorkspaceQuery{Workspace: target}, "").(*protocol.Page[protocol.SkillProposal]).Data
	if len(proposals) != 1 || proposals[0].Origin != protocol.SkillProposalOriginMined || proposals[0].SourceSession != session.ID || proposals[0].Scope != protocol.SkillProposalScopeUser || proposals[0].Revises {
		t.Fatalf("learning proposal = %+v", proposals)
	}
	path := filepath.Join(cfg.skillsUserDir, "verify-workspace", "SKILL.md")
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unapproved skill was published: %v", err)
	}
	proposal := proposals[0]
	r.must(delivery.SkillsProposalsApprove, protocol.SkillProposalRef{Workspace: target, Name: proposal.Name, Revision: proposal.Revision, Scope: proposal.Scope}, "approve-learning")
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "Run go test ./...") {
		t.Fatalf("approved skill = %s, %v", content, err)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-learning")
	for i := range 3 {
		run(i + 3)
	}
	if miningCalls.Load() != 1 {
		t.Fatalf("disabled backend performed mining: %d", miningCalls.Load())
	}
}
