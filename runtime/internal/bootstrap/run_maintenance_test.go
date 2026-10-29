package bootstrap

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	skillspec "github.com/Tangerg/scope/skills"
)

func TestInstalledSkillLearningUsesAdmittedSkillDocuments(t *testing.T) {
	const document = "---\nname: run-tests\ndescription: Run the project tests.\n---\nUse the current test command.\n"
	for _, test := range []struct {
		name     string
		prepare  func(string) error
		category error
	}{
		{name: "valid"},
		{name: "invalid UTF-8", prepare: func(path string) error {
			return os.WriteFile(path, append([]byte(document), 0xff), 0o600)
		}},
		{name: "oversized", prepare: func(path string) error {
			return os.Truncate(path, skills.MaxAuthoredSkillDocumentBytes+1)
		}, category: skills.ErrDocumentTooLarge},
		{name: "directory", prepare: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Mkdir(path, 0o700)
		}},
		{name: "missing", prepare: os.Remove},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := poMCPConfig(t)
			path := filepath.Join(cfg.skillsUserDir, "run-tests", skillspec.SkillFile)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.prepare != nil {
				if err := test.prepare(path); err != nil {
					t.Fatal(err)
				}
			}
			store, err := persistence.NewConversationStore(cfg.stores.ChatHistory)
			if err != nil {
				t.Fatal(err)
			}
			const sessionID = "ses_mining"
			if err := store.Write(t.Context(), sessionID,
				chat.NewUserMessage(chat.NewTextPart("the test command changed")),
				chat.NewAssistantMessage(chat.NewTextPart("use the new command")),
				chat.NewUserMessage(chat.NewTextPart("keep the correction")),
				chat.NewAssistantMessage(chat.NewTextPart("done")),
			); err != nil {
				t.Fatal(err)
			}
			model := &skillRevisionModel{}
			client, err := chatclient.New(model, chatclient.Config{})
			if err != nil {
				t.Fatal(err)
			}
			cfg.chatResolver = testChatResolver(client)
			var learning *pluginservices.SkillLearning
			host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), func(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
				environment, err := buildToolEnvironment(ctx, deps)
				learning = environment.skillLearning
				return environment, err
			})
			if err != nil {
				t.Fatal(err)
			}
			api, err := pluginProtocolHandler(host, cfg)
			if err != nil {
				t.Fatal(err)
			}
			startTestRuntime(t, host, api, cfg.stores)
			r := &poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}
			t.Cleanup(r.close)
			installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "skills"))
			waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
			var failure error
			for range 3 {
				failure = learning.MaintainSkills(t.Context(), executionadapter.RunMaintenanceInput{SessionID: sessionID, WorkspaceCWD: cfg.defaultWorkspacePath, ToolCalls: 8})
			}
			if test.category != nil {
				if !errors.Is(failure, test.category) || !errors.Is(failure, filesystem.ErrFileTooLarge) {
					t.Fatalf("document admission lost its category or cause: %v", failure)
				}
			} else if failure != nil {
				t.Fatal(failure)
			}
			target := protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}
			proposals := r.must(delivery.SkillsProposalsList, protocol.WorkspaceQuery{Workspace: target}, "").(*protocol.Page[protocol.SkillProposal]).Data
			if test.name != "valid" {
				if model.calls != 1 {
					t.Fatalf("unavailable document reached revision generation: %d model calls", model.calls)
				}
				if len(proposals) != 0 {
					t.Fatalf("unavailable document produced revision proposals: %+v", proposals)
				}
				return
			}
			if model.calls != 2 {
				t.Fatalf("valid document did not reach revision generation: %d model calls", model.calls)
			}
			if len(proposals) != 1 || !proposals[0].Revises || proposals[0].Origin != protocol.SkillProposalOriginMined {
				t.Fatalf("mined revision = %+v", proposals)
			}
			current := r.must(delivery.SkillsDiscoveredGet, protocol.SkillDetailRequest{Workspace: target, Name: "run-tests"}, "").(*protocol.SkillDetail)
			if !strings.Contains(current.Instructions, "Use the current test command.") {
				t.Fatalf("mining changed active document before approval: %+v", current)
			}
			proposal := proposals[0]
			r.must(delivery.SkillsProposalsApprove, protocol.SkillProposalRef{Workspace: target, Scope: proposal.Scope, Name: proposal.Name, Revision: proposal.Revision}, "approve")
			current = r.must(delivery.SkillsDiscoveredGet, protocol.SkillDetailRequest{Workspace: target, Name: "run-tests"}, "").(*protocol.SkillDetail)
			if !strings.Contains(current.Instructions, "Run go test ./...") {
				t.Fatalf("approved revision = %+v", current)
			}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-learning")
			for range 3 {
				_ = learning.MaintainSkills(t.Context(), executionadapter.RunMaintenanceInput{SessionID: sessionID, WorkspaceCWD: cfg.defaultWorkspacePath, ToolCalls: 8})
			}
			if model.calls != 2 {
				t.Fatalf("disabled plugin made new model calls: %d", model.calls)
			}

		})
	}
}

type skillRevisionModel struct{ calls int }

func (s *skillRevisionModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	s.calls++
	text := "REVISE: run-tests"
	if s.calls == 2 {
		text = "---\nname: run-tests\ndescription: Run the project tests.\n---\nRun go test ./... with the current workspace."
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (s *skillRevisionModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(s.Call(ctx, request))
}
