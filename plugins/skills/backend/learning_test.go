package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
)

const testSkill = "---\nname: run-tests\ndescription: Run project tests after changing code.\n---\nRun go test ./... from the workspace."

type testInputs struct {
	conversation             skilllearning.Conversation
	skill                    *skilllearning.Skill
	sourceErr                error
	completeErr              error
	replies                  []string
	prompts                  []string
	historyReads, skillReads int
}

func (i *testInputs) Conversation(context.Context) (skilllearning.Conversation, error) {
	i.historyReads++
	return i.conversation, nil
}
func (i *testInputs) ReadSkill(context.Context, string) (*skilllearning.Skill, error) {
	i.skillReads++
	return i.skill, i.sourceErr
}
func (i *testInputs) Complete(_ context.Context, _, input string) (string, error) {
	i.prompts = append(i.prompts, input)
	if i.completeErr != nil {
		return "", i.completeErr
	}
	return i.replies[len(i.prompts)-1], nil
}

func freshLearner() *learner { return &learner{complexRuns: make(map[string]int)} }

func dueMine(t *testing.T, l *learner, inputs learningInputs) (skilllearning.MineResult, error) {
	t.Helper()
	var result skilllearning.MineResult
	var err error
	for range 3 {
		result, err = l.mine(t.Context(), skilllearning.Run{SessionID: "ses_learning", ToolCalls: 8}, inputs)
		if err != nil {
			return result, err
		}
	}
	return result, err
}

func TestLearningCadenceCountsOnlyComplexRunsPerSession(t *testing.T) {
	l := freshLearner()
	i := &testInputs{conversation: skilllearning.Conversation{MessageCount: 4, Transcript: "conversation"}, replies: []string{"NO_SKILL", "NO_SKILL"}}
	for range 20 {
		if _, err := l.mine(t.Context(), skilllearning.Run{SessionID: "ses_learning", ToolCalls: 7}, i); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := l.mine(t.Context(), skilllearning.Run{SessionID: "ses_other", ToolCalls: 8}, i); err != nil {
			t.Fatal(err)
		}
	}
	if i.historyReads != 0 || len(i.prompts) != 0 {
		t.Fatal("routine or non-due runs performed external reads")
	}
	for range 2 {
		if _, err := dueMine(t, l, i); err != nil {
			t.Fatal(err)
		}
	}
	if i.historyReads != 2 || len(i.prompts) != 2 {
		t.Fatalf("due attempts: %+v", i)
	}
}

func TestLearningReturnsOnlyUsableDrafts(t *testing.T) {
	for _, test := range []struct {
		name, reply string
		messages    int64
		proposal    bool
	}{
		{"new", testSkill, 4, true},
		{"fenced", "```markdown\n" + testSkill + "\n```", 4, true},
		{"sentinel", " no_skill\n", 4, false},
		{"invalid", "not a skill document", 4, false},
		{"short conversation", testSkill, 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			i := &testInputs{conversation: skilllearning.Conversation{MessageCount: test.messages, Transcript: "conversation"}, replies: []string{test.reply}}
			result, err := dueMine(t, freshLearner(), i)
			if err != nil || (result.Proposal != nil) != test.proposal {
				t.Fatalf("draft = %+v, %v", result, err)
			}
			if result.Proposal != nil && (result.Proposal.Skill.Name != "run-tests" || result.Proposal.Revises) {
				t.Fatalf("new draft = %+v", result)
			}
			if test.messages < 4 && len(i.prompts) != 0 {
				t.Fatal("short conversation dispatched model")
			}
		})
	}
}

func TestLearningRevisionReadsCurrentDocumentAndCannotRenameIt(t *testing.T) {
	i := &testInputs{conversation: skilllearning.Conversation{MessageCount: 4, Transcript: "correct the test command"}, skill: &skilllearning.Skill{Name: "run-tests", Description: "Run tests", Instructions: "ACTUAL CURRENT PROCEDURE"}, replies: []string{"REVISE: run-tests", strings.Replace(testSkill, "run-tests", "another-name", 1)}}
	result, err := dueMine(t, freshLearner(), i)
	if err != nil || result.Proposal == nil || !result.Proposal.Revises || result.Proposal.Skill.Name != "run-tests" {
		t.Fatalf("revision = %+v, %v", result, err)
	}
	if i.skillReads != 1 || len(i.prompts) != 2 || !strings.Contains(i.prompts[1], i.skill.Instructions) || !strings.Contains(i.prompts[1], i.conversation.Transcript) {
		t.Fatalf("revision source: %+v", i)
	}
}

func TestLearningMissingSourceAndFailuresDoNotBecomeDrafts(t *testing.T) {
	failure := errors.New("source failed")
	for _, sourceErr := range []error{nil, failure} {
		i := &testInputs{conversation: skilllearning.Conversation{MessageCount: 4}, replies: []string{"REVISE: run-tests"}, sourceErr: sourceErr}
		result, err := dueMine(t, freshLearner(), i)
		if !errors.Is(err, sourceErr) || result.Proposal != nil || len(i.prompts) != 1 {
			t.Fatalf("missing source = %+v, %v", result, err)
		}
	}
	for _, failure := range []error{context.Canceled, errors.New("model response lost")} {
		i := &testInputs{conversation: skilllearning.Conversation{MessageCount: 4}, completeErr: failure}
		l := freshLearner()
		if result, err := dueMine(t, l, i); !errors.Is(err, failure) || result.Proposal != nil {
			t.Fatalf("model failure = %+v, %v", result, err)
		}
		if _, err := l.mine(t.Context(), skilllearning.Run{SessionID: "ses_learning", ToolCalls: 8}, i); err != nil || len(i.prompts) != 1 {
			t.Fatal("failed attempt did not reset cadence")
		}
	}
}

func TestArchiveAdviceHasOneDecisionPerInterval(t *testing.T) {
	l := freshLearner()
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	var advised atomic.Int32
	var calls sync.WaitGroup
	for range 20 {
		calls.Go(func() {
			if l.archive(now).AfterMillis != nil {
				advised.Add(1)
			}
		})
	}
	calls.Wait()
	if advised.Load() != 1 {
		t.Fatalf("concurrent decisions = %d", advised.Load())
	}
	for _, delta := range []time.Duration{-time.Hour, 0, 6*time.Hour - time.Nanosecond} {
		if l.archive(now.Add(delta)).AfterMillis != nil {
			t.Fatalf("early archive advice at %v", delta)
		}
	}
	advice := l.archive(now.Add(6 * time.Hour))
	if duration, err := advice.Duration(); err != nil || duration != 30*24*time.Hour {
		t.Fatalf("next archive advice = %+v, %v", advice, err)
	}
}
