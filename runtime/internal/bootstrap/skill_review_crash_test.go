package bootstrap

import (
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	skillspec "github.com/Tangerg/scope/skills"
	modernsqlite "modernc.org/sqlite"
)

const (
	skillReviewCrashHome   = "FLAME_TEST_SKILL_REVIEW_HOME"
	skillReviewCrashRunner = "FLAME_TEST_SKILL_REVIEW_RUNNER"
	skillReviewCrashMethod = "FLAME_TEST_SKILL_REVIEW_METHOD"
	skillReviewConflict    = "FLAME_TEST_SKILL_REVIEW_CONFLICT"
	skillReviewConfirmCut  = "FLAME_TEST_SKILL_REVIEW_CONFIRM_CUT"
	skillReviewCrashKey    = "finish-original-review"
	skillReviewFixtureName = "review-fixture"
	skillReviewFixtureFile = "original-review.json"
	skillReviewAuthoredDoc = "---\nname: " + skillReviewFixtureName + "\ndescription: A user-authored Skill.\n---\nKeep the authored document.\n"
)

func TestSkillReviewCrashChild(t *testing.T) {
	home := os.Getenv(skillReviewCrashHome)
	if home == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_skill_review_crash", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	cfg := skillReviewCrashConfig(t, home, os.Getenv(skillReviewCrashRunner))
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "skills"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	ref := stageSkillReview(t, r, cfg, "original-proposal", "Use the original review instructions.")
	if os.Getenv(skillReviewConflict) != "" {
		directory := filepath.Join(cfg.skillsUserDir, ref.Name)
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, skillspec.SkillFile), []byte(skillReviewAuthoredDoc), 0600); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, skillReviewFixtureFile), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	cut := `BEFORE UPDATE OF payload ON idempotency_records
		WHEN OLD.key = '` + skillReviewCrashKey + `' AND NEW.kind = 'response'
		`
	if os.Getenv(skillReviewConfirmCut) != "" {
		confirm, err := skilllearning.ConfirmMethod()
		if err != nil {
			t.Fatal(err)
		}
		cut = `BEFORE UPDATE OF phase ON plugin_commands
			WHEN OLD.method = '` + confirm.Name() + `' AND NEW.phase = '` + string(plugin.CommandDispatched) + `'
			`
	}
	questionCrashDatabase(t, home, `CREATE TRIGGER interrupt_skill_review_acknowledgement `+cut+`BEGIN SELECT test_skill_review_crash(); END`)
	method := delivery.Name(os.Getenv(skillReviewCrashMethod))
	result := r.endpoint.Invoke(t.Context(), method, ref, protocol.CallMetadata{IdempotencyKey: skillReviewCrashKey})
	t.Fatalf("public review receipt was not reached: %v", result.Failure)
}

func TestSkillReviewAcknowledgementSurvivesAbruptExit(t *testing.T) {
	for _, test := range []struct {
		name               string
		method             delivery.Name
		conflict           bool
		beforeConfirmation bool
	}{
		{name: "approved", method: delivery.SkillsProposalsApprove},
		{name: "prepared confirmation", method: delivery.SkillsProposalsApprove, beforeConfirmation: true},
		{name: "refused publication", method: delivery.SkillsProposalsApprove, conflict: true},
		{name: "rejected", method: delivery.SkillsProposalsReject},
	} {
		t.Run(test.name, func(t *testing.T) {
			home, runnerDir := testsupport.TempDirectory(t), testsupport.TempDirectory(t)
			if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSkillReviewCrashChild$", "-test.timeout=35s")
			command.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), skillReviewCrashHome+"="+home,
				skillReviewCrashRunner+"="+runnerDir, skillReviewCrashMethod+"="+test.method.String())
			if test.conflict {
				command.Env = append(command.Env, skillReviewConflict+"=1")
			}
			if test.beforeConfirmation {
				command.Env = append(command.Env, skillReviewConfirmCut+"=1")
			}
			command.WaitDelay = 3 * time.Second
			output, err := command.CombinedOutput()
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
				t.Fatalf("review process did not stop before its acknowledgement: %v\n%s", err, output)
			}
			questionCrashDatabase(t, home, "DROP TRIGGER interrupt_skill_review_acknowledgement")
			encoded, err := os.ReadFile(filepath.Join(home, skillReviewFixtureFile))
			if err != nil {
				t.Fatal(err)
			}
			var original protocol.SkillProposalRef
			if err := json.Unmarshal(encoded, &original); err != nil {
				t.Fatal(err)
			}
			cfg := skillReviewCrashConfig(t, home, runnerDir)
			r := poMCPOpen(t, cfg)
			installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation]).Data
			if len(installations) != 1 {
				t.Fatalf("restored review provider = %+v", installations)
			}
			waitCallContributorState(t, r, installations[0].ID, protocol.MCPServerConnected)
			query := protocol.WorkspaceQuery{Workspace: original.Workspace}
			items := r.must(delivery.SkillsProposalsList, query, "").(*protocol.Page[protocol.SkillProposal]).Data
			if test.beforeConfirmation {
				if len(items) != 1 || items[0].Revision != original.Revision {
					t.Fatalf("prepared confirmation should still own the open proposal: %+v", items)
				}
			} else if len(items) != 0 {
				t.Fatalf("the original decision did not close the proposal: %+v", items)
			}
			path := filepath.Join(cfg.skillsUserDir, original.Name, skillspec.SkillFile)
			if test.conflict {
				result := r.endpoint.Invoke(t.Context(), test.method, original, protocol.CallMetadata{IdempotencyKey: skillReviewCrashKey})
				if !errors.Is(result.Failure, protocol.ErrRevisionConflict) {
					t.Fatalf("publication refusal replay = %v", result.Failure)
				}
				if content, err := os.ReadFile(path); err != nil || string(content) != skillReviewAuthoredDoc {
					t.Fatalf("publication refusal replaced authored document: %q, %v", content, err)
				}
				return
			}
			if test.method == delivery.SkillsProposalsApprove {
				if content, err := os.ReadFile(path); err != nil || !strings.Contains(string(content), "Use the original review instructions.") {
					t.Fatalf("original approved document = %q, %v", content, err)
				}
				const later = "---\nname: review-fixture\ndescription: A later user edit.\n---\nUse the later instructions.\n"
				if err := os.WriteFile(path, []byte(later), 0600); err != nil {
					t.Fatal(err)
				}
				const laterArchive = "later-archive-awaiting-receipt"
				questionCrashDatabase(t, home, `CREATE TRIGGER refuse_later_skill_archive
					BEFORE UPDATE OF payload ON idempotency_records WHEN OLD.key = '`+laterArchive+`'
					BEGIN SELECT RAISE(ABORT, 'later archive receipt withheld'); END`)
				archive := r.endpoint.Invoke(t.Context(), delivery.SkillsLibraryArchive, protocol.SkillNameRequest{Name: original.Name}, protocol.CallMetadata{IdempotencyKey: laterArchive})
				if !errors.Is(archive.Failure, protocol.ErrIdempotencyInProgress) {
					t.Fatalf("later archive should await its own receipt: %v", archive.Failure)
				}
				path = filepath.Join(cfg.skillsUserDir, "_archive", original.Name, skillspec.SkillFile)
				defer func() {
					if content, err := os.ReadFile(path); err != nil || string(content) != later {
						t.Errorf("public approval replay rewrote the later document: %q, %v", content, err)
					}
				}()
			} else {
				replacement := stageSkillReview(t, r, cfg, "new-proposal", "Keep the replacement review open.")
				defer func() {
					items := r.must(delivery.SkillsProposalsList, query, "").(*protocol.Page[protocol.SkillProposal]).Data
					if len(items) != 1 || items[0].Revision != replacement.Revision {
						t.Errorf("public rejection replay changed another proposal: %+v", items)
					}
				}()
			}
			r.must(test.method, original, skillReviewCrashKey)
			if test.beforeConfirmation {
				if items := r.must(delivery.SkillsProposalsList, query, "").(*protocol.Page[protocol.SkillProposal]).Data; len(items) != 0 {
					t.Fatalf("the resumed confirmation did not close the proposal: %+v", items)
				}
			}
		})
	}
}

func stageSkillReview(t *testing.T, r *poMCPRuntime, cfg assemblyConfig, key, instructions string) protocol.SkillProposalRef {
	t.Helper()
	ctx := operation.Start(t.Context(), extension.Model, "test-skill-review", key)
	ref, err := r.host.application.delivery.WorkspaceSkills.SubmitProposal(ctx, cfg.defaultWorkspacePath, skills.Proposal{
		Scope: skills.ScopeUser, Name: skillReviewFixtureName, Description: "Keep an exact Skill review outcome.",
		Instructions: instructions, Origin: skills.ProposalOriginRequested,
	})
	if err != nil {
		t.Fatal(err)
	}
	return protocol.SkillProposalRef{Workspace: protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}, Scope: protocol.SkillProposalScopeUser, Name: ref.Name, Revision: ref.Revision}
}

func skillReviewCrashConfig(t *testing.T, home, runnerDir string) assemblyConfig {
	t.Helper()
	cfg := externalMCPRestartConfig(t, home, runnerDir)
	cfg.userHome = cfg.stores.DataDirectory
	cfg.defaultWorkspacePath = cfg.stores.DataDirectory
	cfg.skillsUserDir = filepath.Join(cfg.stores.DataDirectory, "skills")
	return cfg
}
