package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func connectLearning(t *testing.T, path string) (*sdk.ClientSession, *qimumcp.Binding, func()) {
	t.Helper()
	learner, err := openLearner(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(learner)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "skill-learning-test", Version: "1"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	close := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), connection.Close(), learner.db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(close)
	contract, err := skilllearning.Contract()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := qimumcp.ResolveCapability(t.Context(), session, "skill-learning", contract)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := skilllearning.MineMethod()
	if err != nil {
		t.Fatal(err)
	}
	method, err := exported.Method(mine.Name())
	if err != nil {
		t.Fatal(err)
	}
	return session, method, close
}

func learningInvocation(t *testing.T, id string) context.Context {
	t.Helper()
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: id, Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "skills"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestLearningReceiptsSurviveRestartWithoutRepeatingModelCalls(t *testing.T) {
	for _, outcome := range []string{"completed", "response lost", "incomplete", "refused"} {
		t.Run(outcome, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "learning.db")
			session, mine, closeBackend := connectLearning(t, path)
			callbacks := qimumcp.NewCallbacks()
			t.Cleanup(func() { _ = callbacks.Close() })
			if err := callbacks.AdmitSession(session); err != nil {
				t.Fatal(err)
			}
			var modelCalls atomic.Int32
			model, err := pluginsdk.NewModelCallback(func(context.Context, qimumcp.Command[chat.Request]) (*chat.Response, error) {
				modelCalls.Add(1)
				if outcome == "response lost" {
					return nil, errors.New("model reply lost")
				}
				finish := chat.FinishReasonStop
				if outcome == "incomplete" {
					finish = chat.FinishReasonLength
				}
				if outcome == "refused" {
					finish = chat.FinishReasonRefusal
				}
				message := chat.NewAssistantMessage(chat.NewTextPart(testSkill))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, nil)
			})
			if err != nil {
				t.Fatal(err)
			}
			historyMethod, err := skilllearning.ConversationMethod()
			if err != nil {
				t.Fatal(err)
			}
			history, err := qimumcp.NewCallback(historyMethod, nil, func(context.Context, qimumcp.Command[struct{}]) (skilllearning.Conversation, error) {
				return skilllearning.Conversation{MessageCount: 4, Transcript: "retained conversation"}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx := learningInvocation(t, "maintenance")
			grant, release, err := callbacks.Open(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: skilllearning.MaxCallbackBytes, MaxResponseBytes: skilllearning.MaxCallbackBytes}, model, history)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			metadata, err := qimumcp.CallbackMetadata(grant)
			if err != nil {
				t.Fatal(err)
			}
			input, err := jsonvalue.Encode(skilllearning.Run{SessionID: "ses_receipt", ToolCalls: 8})
			if err != nil {
				t.Fatal(err)
			}
			for n := range 2 {
				if _, err := mine.Invoke(ctx, fmt.Sprintf("cadence-%d", n), input); err != nil {
					t.Fatal(err)
				}
			}
			value, err := mine.InvokeWithMetadata(ctx, "due", input, metadata)
			complete := outcome == "completed"
			if complete != (err == nil) {
				t.Fatalf("mining outcome = %s, %v", value.JSON(), err)
			}
			if complete {
				var result skilllearning.MineResult
				if err := json.Unmarshal(value.JSON(), &result); err != nil || result.Proposal == nil || result.Proposal.Skill.Name != "run-tests" {
					t.Fatalf("draft = %+v, %v", result, err)
				}
			}
			release()
			for range 2 {
				if repeated, err := mine.Invoke(ctx, "due", input); complete != (err == nil) || complete && repeated != value {
					t.Fatalf("repeated outcome = %s, %v", repeated.JSON(), err)
				}
			}
			closeBackend()
			_, restarted, _ := connectLearning(t, path)
			if repeated, err := restarted.Invoke(ctx, "due", input); complete != (err == nil) || complete && repeated != value {
				t.Fatalf("restart outcome = %s, %v", repeated.JSON(), err)
			}
			receipt, found, err := restarted.ReadReceipt(ctx, "due", input)
			if err != nil || found != complete || complete && receipt != value {
				t.Fatalf("receipt = %s, %t, %v", receipt.JSON(), found, err)
			}
			if modelCalls.Load() != 1 {
				t.Fatalf("replayed model calls = %d", modelCalls.Load())
			}
			changed, err := jsonvalue.Encode(skilllearning.Run{SessionID: "ses_other", ToolCalls: 8})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.Invoke(ctx, "due", changed); err == nil {
				t.Fatal("same command accepted different input")
			}
		})
	}
}
