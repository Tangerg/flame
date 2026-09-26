package workbench

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPendingResumePreservesMixedMemberReviewsAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_child_a", ItemID: "item_approval", Title: "Approve child",
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "printf approved", Status: agent.ToolRunning},
	}
	question := agent.Question{
		RunID: "run_child_b", ItemID: "item_question", Title: "Choose child strategy",
		Fields: []agent.QuestionField{{Prompt: "Strategy", Kind: agent.QuestionSingle, Options: []protocol.QuestionOption{{Label: "Safe"}, {Label: "Fast"}}}},
	}
	pending := PendingResume{
		Command: agent.ResumeRun{
			CommandID: "cli_33333333333333333333333333333333", RunID: "run_root",
			Answers: []agent.InterruptAnswer{
				{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: protocol.ApprovalApprove}},
				{ItemID: question.ItemID, Answer: agent.QuestionAnswer{Values: [][]string{{"Safe"}}}},
			},
		},
		Interactions: []agent.Interaction{approval, question}, Replay: commandreplay.UnprotectedGuard(),
	}
	if err := store.StagePendingResume("ses_1", pending, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, ok := reopened.PendingResume("ses_1")
	if !ok || !pendingResumeEqual(restored, pending) || restored.Command.RunID != "run_root" ||
		agent.InteractionRunID(restored.Interactions[0]) != approval.RunID || agent.InteractionRunID(restored.Interactions[1]) != question.RunID {
		t.Fatalf("restored root/member decision = %+v, found=%t", restored, ok)
	}
	for _, mutate := range []func(*PendingResume){
		func(p *PendingResume) { p.Command.Answers = p.Command.Answers[:1] },
		func(p *PendingResume) { p.Command.Answers[1].ItemID = p.Command.Answers[0].ItemID },
		func(p *PendingResume) { p.Command.Answers[1].Answer = agent.QuestionAnswer{} },
	} {
		invalid := clonePendingResume(pending)
		mutate(&invalid)
		if err := invalid.validate(); err == nil {
			t.Fatalf("accepted an incomplete or duplicate member answer: %+v", invalid.Command.Answers)
		}
	}
}
