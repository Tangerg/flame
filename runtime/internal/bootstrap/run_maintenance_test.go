package bootstrap

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	skillspec "github.com/Tangerg/scope/skills"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestDefaultMaintenanceUsesAdmittedSkillDocuments(t *testing.T) {
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
			cfg := runtimeConfigWithRequiredDeps(t)
			path := filepath.Join(cfg.SkillsUserDir, "run-tests", skillspec.SkillFile)
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
			workspace, err := buildWorkspaceComposition(cfg, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			store, err := persistence.NewConversationStore(cfg.Stores.ChatHistory)
			if err != nil {
				t.Fatal(err)
			}
			compactions, err := persistence.NewConversationCompactions(store, cfg.Stores.Runs, persistence.Transactor(cfg.Stores.Transactor))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err := buildConversationEnvironment(store, compactions)
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
			resolve := modeladapter.AuxiliaryResolver(func(context.Context, modeladapter.AuxiliaryBudget) (modeladapter.SelectedChat, func(), error) {
				return auxiliaryFixture(model), func() {}, nil
			})
			pipeline, _, err := buildRunMaintenance(cfg, conversation, nil, workspace.skills, workspace.skillMaintenance, noMemoryMaintenance{}, resolve)
			if err != nil {
				t.Fatal(err)
			}
			var result executionadapter.RunMaintenanceResult
			for range 3 {
				result = pipeline.Maintain(t.Context(), executionadapter.RunMaintenanceInput{
					SessionID: sessionID, WorkspaceCWD: cfg.DefaultWorkspacePath, ToolCalls: 8,
				})
			}
			failure := errors.Join(result.Errors...)
			if test.category != nil {
				if !errors.Is(failure, test.category) || !errors.Is(failure, fileinput.ErrTooLarge) {
					t.Fatalf("document admission lost its category or cause: %v", failure)
				}
			} else if failure != nil {
				t.Fatal(failure)
			}
			proposals, err := workspace.skills.Proposals(t.Context(), cfg.DefaultWorkspacePath)
			if err != nil {
				t.Fatal(err)
			}
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
			if len(proposals) != 1 || !proposals[0].Revises || proposals[0].Origin != skills.ProposalOriginMined {
				t.Fatalf("mined revision = %+v, %v", proposals, err)
			}
			current, err := workspace.skills.Get(t.Context(), cfg.DefaultWorkspacePath, "run-tests")
			if err != nil || !strings.Contains(current.Instructions, "Use the current test command.") {
				t.Fatalf("mining changed the active document before approval: %+v, %v", current, err)
			}
			if err := workspace.skills.ApproveProposal(t.Context(), cfg.DefaultWorkspacePath, proposals[0].Ref); err != nil {
				t.Fatal(err)
			}
			current, err = workspace.skills.Get(t.Context(), cfg.DefaultWorkspacePath, "run-tests")
			if err != nil || !strings.Contains(current.Instructions, "Run go test ./...") {
				t.Fatalf("approved revision = %+v, %v", current, err)
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

type noMemoryMaintenance struct{}

func (noMemoryMaintenance) MaintainMemory(context.Context, executionadapter.RunMaintenanceInput) error {
	return nil
}
