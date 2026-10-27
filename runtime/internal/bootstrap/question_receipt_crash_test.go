package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

func acceptedQuestionModel() chat.Model {
	return delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		results := 0
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult == nil {
					continue
				}
				results++
				if len(part.ToolResult.Output.Content) != 1 || !strings.Contains(part.ToolResult.Output.Content[0].Text, "chosen") {
					return nil, errors.New("accepted question answer changed after receipt recovery")
				}
			}
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
		finish := chat.FinishReasonStop
		if results == 0 {
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "question", Name: "ask_user", Arguments: `{"questions":[{"question":"Which value?"}]}`}))
			finish = chat.FinishReasonToolCalls
		} else if results != 1 {
			return nil, fmt.Errorf("accepted question results = %d, want exactly one", results)
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
}

func TestAcceptedQuestionReceiptCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_ACCEPTED_QUESTION_RECEIPTS")
	if phase == "" {
		return
	}
	want, err := strconv.Atoi(phase)
	if err != nil {
		t.Fatal(err)
	}
	if err := modernsqlite.RegisterScalarFunction("test_question_receipt_crash", 1, func(_ *modernsqlite.FunctionContext, values []driver.Value) (driver.Value, error) {
		var payload []byte
		switch value := values[0].(type) {
		case []byte:
			payload = value
		case string:
			payload = []byte(value)
		default:
			return nil, fmt.Errorf("unexpected execution tree payload %T", value)
		}
		if want == 1 && strings.Contains(string(payload), "answer:") {
			os.Exit(commandCrashExit)
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	if want == 0 {
		tx := stores.Transactor
		stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
			if err := tx(ctx, apply); err != nil {
				return err
			}
			accepted, err := stores.AcceptedContinuations.List(ctx)
			if err == nil && len(accepted) == 1 {
				os.Exit(commandCrashExit)
			}
			return err
		}
	}
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, acceptedQuestionModel()), home)
	defer host.Close()
	options := acceptedCommandOptions()
	options.RequestMeta.ClientCapabilities.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion}
	ctx := delivery.WithRequestMeta(t.Context(), options.RequestMeta)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "ask a question"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range events {
		if err != nil {
			t.Fatal(err)
		}
	}
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("pending questions = %+v, %v", pending, err)
	}
	if want == 1 {
		questionCrashDatabase(t, home, `CREATE TRIGGER question_receipt_crash BEFORE UPDATE ON execution_trees BEGIN SELECT test_question_receipt_crash(OLD.payload); END`)
	}
	request := protocol.ResumeRunRequest{RunID: started.RunID}
	for _, question := range pending.Data[0].Interrupts {
		request.Responses = append(request.Responses, protocol.InterruptResponse{ItemID: question.ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"chosen"}}}})
	}
	result := host.Endpoint().Invoke(ctx, delivery.RunsResume, request, options)
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	for _, err := range result.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("question receipt crash was not reached")
}

func questionCrashDatabase(t *testing.T, home, statement string) {
	t.Helper()
	directory, err := localruntime.DataDirectoryAt(home)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), statement)
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptedQuestionSurvivesAcceptanceAndAnswerReceiptCrash(t *testing.T) {
	for _, count := range []int{0, 1} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcceptedQuestionReceiptCrashHelper$")
			command.Env = append(os.Environ(), "FLAME_TEST_ACCEPTED_QUESTION_RECEIPTS="+strconv.Itoa(count))
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
				t.Fatalf("question receipt crash = %v\n%s", err, output)
			}
			if count == 1 {
				questionCrashDatabase(t, home, `DROP TRIGGER question_receipt_crash`)
			}
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stores.Close() })
			bindings, err := stores.AcceptedContinuations.List(t.Context())
			if err != nil || len(bindings) != 1 {
				t.Fatalf("accepted continuation = %+v, %v", bindings, err)
			}
			accepted := bindings[0]
			host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, acceptedQuestionModel()), home)
			t.Cleanup(func() { _ = host.Close() })
			waitForProtocolRunTerminal(t, t.Context(), api, accepted.RootRunID())
			value, err := api.GetRun(t.Context(), protocol.GetRunRequest{RunID: accepted.RootRunID()})
			if err != nil || value.Outcome == nil || value.Outcome.Type != protocol.OutcomeCompleted {
				t.Fatalf("recovered question Run = %+v, %v", value, err)
			}
			items, err := api.ListItems(t.Context(), protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: accepted.SessionID}})
			if err != nil {
				t.Fatal(err)
			}
			answered := 0
			for _, item := range items.Data {
				if item.Question != nil && len(item.Question.Answers) == 1 && len(item.Question.Answers[0]) == 1 && item.Question.Answers[0][0] == "chosen" {
					answered++
				}
			}
			if answered != 1 {
				t.Fatalf("accepted Question Items = %d, want one", answered)
			}
		})
	}
}
