package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/qimu/mcp"
)

const miningCrashExit = 93
const miningCrashPhase = "FLAME_TEST_MINING_CRASH_PHASE"
const miningCrashDB = "FLAME_TEST_MINING_CRASH_DB"

func TestMiningCrashAtomicallyPreservesProposalAndOriginalReceipt(t *testing.T) {
	method, err := skilllearning.MineMethod()
	if err != nil {
		t.Fatal(err)
	}
	run := skilllearning.Run{Library: "captured-user-library", SessionID: "ses_source", ToolCalls: 8}
	input, err := jsonvalue.Encode(run)
	if err != nil {
		t.Fatal(err)
	}
	const id = "original-mining-command"
	result := skilllearning.MineResult{Proposal: &skilllearning.Proposal{Skill: skilllearning.Skill{Name: "captured", Description: "Check the workspace.", Instructions: "Run the checks."}}}
	if phase := os.Getenv(miningCrashPhase); phase != "" {
		l, err := openLearner(t.Context(), os.Getenv(miningCrashDB))
		if err != nil {
			t.Fatal(err)
		}
		cmd, _, err := storage.AdmitCommand(t.Context(), l.db, method.Name(), id, input, nil)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := qimumcp.EncodeReply(result, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = cmd.SettleExternal(t.Context(), reply.JSON(), func(ctx context.Context) error {
			if err := l.recordMined(ctx, id, run, result); err != nil {
				return err
			}
			if phase == "before-commit" {
				os.Exit(miningCrashExit)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// Exit before a transport can acknowledge the locally committed command.
		os.Exit(miningCrashExit)
	}
	for _, phase := range []string{"before-commit", "after-commit"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "skills.db")
			child := exec.Command(os.Args[0], "-test.run=^TestMiningCrashAtomicallyPreservesProposalAndOriginalReceipt$")
			child.Env = append(os.Environ(), miningCrashPhase+"="+phase, miningCrashDB+"="+path)
			out, err := child.CombinedOutput()
			e, ok := errors.AsType[*exec.ExitError](err)
			if !ok || e.ExitCode() != miningCrashExit {
				t.Fatalf("child did not crash at boundary: %s %v", out, err)
			}
			l, err := openLearner(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.db.Close()
			pending, err := (reviewQueue{db: l.db}).pending(t.Context(), skilllearning.Libraries{User: run.Library})
			if err != nil {
				t.Fatal(err)
			}
			command, reply, err := storage.AdmitCommand(t.Context(), l.db, method.Name(), id, input, func(context.Context) error { t.Fatal("recovery repeated mining admission"); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if phase == "before-commit" {
				if len(reply) != 0 || command == nil || command.NewlyAdmitted() || len(pending.Proposals) != 0 {
					t.Fatal("uncommitted mining exposed a proposal or authorized a second model call")
				}
				return
			}
			got, err := qimumcp.DecodeReply[skilllearning.MineResult](reply)
			if err != nil || got.Proposal == nil || *got.Proposal != *result.Proposal || len(pending.Proposals) != 1 || pending.Proposals[0].Command != id {
				t.Fatalf("original receipt and queue diverged: %+v %+v %v", pending, got, err)
			}
		})
	}
}
