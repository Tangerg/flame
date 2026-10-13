package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const answerCrashExit = 74

type answerFixtureTrees struct {
	runs.ExecutionTreeStore
	results *testExecutionTrees
}

func (s answerFixtureTrees) ExecutionResultCommitted(ctx context.Context, sessionID string, publication runs.ResultPublication) (bool, error) {
	return s.results.ExecutionResultCommitted(ctx, sessionID, publication)
}

type answerCrashTrees struct {
	runs.ExecutionTreeStore
	count int
}

func (s answerCrashTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := s.ExecutionTreeStore.SaveExecutionTree(ctx, update); err != nil {
		return err
	}
	tree, err := agent.ParseTreeSnapshot(update.Head.Payload)
	if err != nil {
		return err
	}
	count := 0
	for _, snapshot := range tree.ProcessSnapshots() {
		for _, receipt := range snapshot.SignalReceipts() {
			if strings.HasPrefix(receipt.ID().String(), "answer:") {
				count++
			}
		}
	}
	if count == s.count {
		os.Exit(answerCrashExit)
	}
	return nil
}

func answerCrashExecutor(t *testing.T, trees runs.ExecutionTreeStore, workspace string) *InteractionExecutor {
	t.Helper()
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "ask", Description: "Ask for a value."}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := runinput.Require(ctx, "crash.question", runs.Interrupt{Kind: interrupt.Question, Question: &runs.QuestionPrompt{ToolName: "ask", Arguments: `{}`, Fields: []runs.QuestionFieldSpec{{Prompt: "Which value?"}}}})
		if err != nil {
			return "", err
		}
		file, err := os.OpenFile(filepath.Join(workspace, "effects.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		_, writeErr := fmt.Fprintln(file, resolution.Answers[0][0])
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return "", err
		}
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolBatchResponse([]chat.ToolCall{{ID: "a", Name: "ask", Arguments: `{}`}, {ID: "b", Name: "ask", Arguments: `{}`}}, 2, 1), interactionUsageTextResponse("done", 3, 1),
	}}
	if os.Getenv("FLAME_TEST_ANSWER_CRASH") == "" {
		model.responses = model.responses[1:]
	}
	return newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ExecutionTrees: trees, MaxConcurrentToolCalls: new(2), ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{concurrentInteractionTool{Tool: question}}}}, ToolInterpreter: immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
}

func checkpointAnswers(t *testing.T, checkpoint runs.ExecutorCheckpoint) []runs.InterruptAnswer {
	t.Helper()
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	var answers []runs.InterruptAnswer
	for waitID, target := range state.handoff.inputs {
		answers = append(answers, runs.InterruptAnswer{InterruptItemID: "item_" + waitID.String(), MemberID: target.memberID.String(), RequestID: waitID.String(), Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}}})
	}
	return answers
}

func TestAnswerDeliveryCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_ANSWER_CRASH")
	if phase == "" {
		return
	}
	count, err := strconv.Atoi(phase)
	if err != nil {
		t.Fatal(err)
	}
	workspace := os.Getenv("FLAME_TEST_ANSWER_WORKSPACE")
	db, err := sqlite.Open(t.Context(), filepath.Join(workspace, "tree.db"))
	if err != nil {
		t.Fatal(err)
	}
	store := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	executor := answerCrashExecutor(t, answerCrashTrees{ExecutionTreeStore: store, count: count}, workspace)
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	barrier := captureAcceptedTestBarrier(t, executor, ref, 2)
	checkpoint := barrier.Checkpoint()
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(checkpoint, workspace, ref.ExecutorID, run.Capabilities{InterruptKinds: start.InterruptKinds})); err != nil {
		t.Fatal(err)
	}
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := session.prepareContinuationAnswers(t.Context(), checkpointAnswers(t, checkpoint))
	if err != nil {
		t.Fatal(err)
	}
	err = session.deliverContinuationAnswers(t.Context(), prepared, nil)
	t.Fatalf("answer crash was not reached: %v", err)
}

func TestAnswerReceiptsSurviveAbruptRestart(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			workspace := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAnswerDeliveryCrashHelper$")
			command.Env = append(os.Environ(), "FLAME_TEST_ANSWER_CRASH="+strconv.Itoa(count), "FLAME_TEST_ANSWER_WORKSPACE="+workspace)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != answerCrashExit {
				t.Fatalf("crash=%v\n%s", err, output)
			}
			if _, err := os.Stat(filepath.Join(workspace, "effects.txt")); !os.IsNotExist(err) {
				t.Fatalf("paused Tool executed: %v", err)
			}
			db, err := sqlite.Open(t.Context(), filepath.Join(workspace, "tree.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			var rootID string
			if err := db.QueryRowContext(t.Context(), `SELECT root_member_id FROM executor_checkpoints`).Scan(&rootID); err != nil {
				t.Fatal(err)
			}
			store := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
			checkpoint, err := store.LoadCheckpoint(t.Context(), rootID)
			if err != nil {
				t.Fatal(err)
			}
			executor := answerCrashExecutor(t, answerFixtureTrees{ExecutionTreeStore: store, results: testTrees(t)}, workspace)
			answers := checkpointAnswers(t, checkpoint)
			ref, err := executor.StageAcceptedContinuation(t.Context(), rootInteractionWaitingContinuation(checkpoint, workspace, "exec_restored", run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}), answers)
			if err != nil {
				t.Fatal(err)
			}
			session, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := session.prepareContinuationAnswers(t.Context(), answers)
			if err != nil {
				t.Fatal(err)
			}
			admitted := 0
			for _, answer := range prepared {
				if answer.admitted {
					admitted++
				}
			}
			if admitted != count {
				t.Fatalf("durable receipts=%d,want %d", admitted, count)
			}
			sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			events := collectInteractionEvents(sequence)
			if err := executor.BeginContinuation(t.Context(), ref, answers, nil); err != nil {
				t.Fatal(err)
			}
			completed := <-events
			if len(payloadsOf[runs.ToolResultsCommitted](completed)) != 2 {
				t.Fatalf("Tool results=%+v", completed)
			}
			content, err := os.ReadFile(filepath.Join(workspace, "effects.txt"))
			if err != nil || string(content) != "chosen\nchosen\n" {
				t.Fatalf("Tool effects=%q,%v", content, err)
			}
		})
	}
}
