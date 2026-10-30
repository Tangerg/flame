package pluginservices

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	skillspec "github.com/Tangerg/scope/skills"
)

type learningSourceFunc func(context.Context, string) (*skillspec.Skill, error)

func (f learningSourceFunc) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	return f(ctx, name)
}

type learningHistoryFunc func(context.Context, string) ([]chat.Message, error)

func (f learningHistoryFunc) Read(ctx context.Context, session string) ([]chat.Message, error) {
	return f(ctx, session)
}

func openLearningGrant(t *testing.T, ctx context.Context, owner *SkillLearning) (qimumcp.CallbackRef, *learningAdmission, func()) {
	t.Helper()
	owner.callbacks = qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = owner.callbacks.Close() })
	session := new(sdk.ClientSession)
	if err := owner.callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	state := &learningAdmission{modelCalls: make(map[string]jsonvalue.Value)}
	grant, release, err := owner.grant(ctx, session, "ses_owner", state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return grant, state, release
}

func TestLearningGrantBoundsSourceReadsAndRetainsModelAncestry(t *testing.T) {
	var calls, releases atomic.Int32
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "skills"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := &SkillLearning{
		history: learningHistoryFunc(func(_ context.Context, session string) ([]chat.Message, error) {
			if session != "ses_owner" {
				t.Errorf("history escaped original session: %s", session)
			}
			return []chat.Message{chat.NewUserMessage(chat.NewTextPart("original conversation"))}, nil
		}),
		source: learningSourceFunc(func(_ context.Context, name string) (*skillspec.Skill, error) {
			return &skillspec.Skill{Name: name, Description: "current", Instructions: "read body"}, nil
		}),
		utility: func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func() error, error) {
			return auxiliaryFixture(textModel(func(ctx context.Context, _ *chat.Request) (string, error) {
				invocation, err := qimumcp.InvocationFromContext(ctx)
				if err != nil || invocation.Parent != "original" || invocation.Origin != extension.Automatic || invocation.ID == "original" {
					t.Errorf("model ancestry = %+v, %v", invocation, err)
				}
				calls.Add(1)
				return "NO_SKILL", nil
			})), func() error { releases.Add(1); return nil }, nil
		},
	}
	grant, state, release := openLearningGrant(t, ctx, owner)
	history, err := skilllearning.ConversationMethod()
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := qimumcp.CallHost(t.Context(), grant, history, "", struct{}{})
	if err != nil || conversation.MessageCount != 1 || !strings.Contains(conversation.Transcript, "original conversation") {
		t.Fatalf("history = %+v, %v", conversation, err)
	}
	if _, err := qimumcp.CallHost(t.Context(), grant, history, "", struct{}{}); err == nil {
		t.Fatal("history reread accepted")
	}
	read, err := skilllearning.ReadSkillMethod()
	if err != nil {
		t.Fatal(err)
	}
	current, err := qimumcp.CallHost(t.Context(), grant, read, "", skilllearning.ReadSkill{Name: "run-tests"})
	if err != nil || current.Skill == nil || current.Skill.Instructions != "read body" {
		t.Fatalf("source = %+v, %v", current, err)
	}
	if _, err := qimumcp.CallHost(t.Context(), grant, read, "", skilllearning.ReadSkill{Name: "other-skill"}); err == nil {
		t.Fatal("second source read accepted")
	}
	if name, _ := state.outcome(); name != "run-tests" {
		t.Fatalf("admitted source = %s", name)
	}
	for _, id := range []string{"first", "second"} {
		if _, err := pluginsdk.CallModel(t.Context(), grant, id, testModelRequest()); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"first", "third"} {
		if _, err := pluginsdk.CallModel(t.Context(), grant, id, testModelRequest()); err == nil {
			t.Fatalf("model invocation %s escaped its budget", id)
		}
	}
	changed := testModelRequest()
	changed.Messages[1] = chat.NewUserMessage(chat.NewTextPart("different request"))
	if _, err := pluginsdk.CallModel(t.Context(), grant, "first", changed); err == nil {
		t.Fatal("changed model invocation was admitted")
	}
	release()
	if _, err := pluginsdk.CallModel(t.Context(), grant, "after-release", testModelRequest()); err == nil {
		t.Fatal("released grant remained callable")
	}
	if calls.Load() != 2 || releases.Load() != 2 {
		t.Fatalf("model ownership calls=%d releases=%d", calls.Load(), releases.Load())
	}
}

func TestLearningGrantRejectsForeignModelEnvelopesBeforeResolution(t *testing.T) {
	var resolutions atomic.Int32
	owner := &SkillLearning{utility: func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func() error, error) {
		resolutions.Add(1)
		return auxiliaryFixture(textModel(func(context.Context, *chat.Request) (string, error) { return "NO_SKILL", nil })), func() error { return nil }, nil
	}}
	grant, _, _ := openLearningGrant(t, t.Context(), owner)
	for _, edit := range []func(*chat.Request){
		func(r *chat.Request) { r.Options.Model = "foreign" },
		func(r *chat.Request) { r.Options.MaxOutputTokens = new(skilllearning.MaxModelOutputTokens + 1) },
		func(r *chat.Request) {
			r.Messages[1] = chat.NewUserMessage(chat.NewTextPart(strings.Repeat("x", skilllearning.MaxModelInputBytes)))
		},
		func(r *chat.Request) {
			r.Messages[1] = chat.NewAssistantMessage(chat.NewTextPart("not the granted role"))
		},
	} {
		request := testModelRequest()
		edit(request)
		if _, err := pluginsdk.CallModel(t.Context(), grant, "rejected", request); err == nil {
			t.Fatal("foreign model envelope was accepted")
		}
	}
	if resolutions.Load() != 0 {
		t.Fatal("invalid envelope acquired a model")
	}
	if _, err := pluginsdk.CallModel(t.Context(), grant, "valid", testModelRequest()); err != nil {
		t.Fatal(err)
	}
}

func TestLearningGrantCancellationJoinsAndReleasesCapturedModel(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("maintenance canceled")
	var releases atomic.Int32
	owner := &SkillLearning{utility: func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func() error, error) {
		return auxiliaryFixture(textModel(func(ctx context.Context, _ *chat.Request) (string, error) {
			close(started)
			<-ctx.Done()
			return "", errors.Join(ctx.Err(), context.Cause(ctx))
		})), func() error { releases.Add(1); return nil }, nil
	}}
	grant, state, release := openLearningGrant(t, ctx, owner)
	done := make(chan error, 1)
	go func() { _, err := pluginsdk.CallModel(t.Context(), grant, "cancel", testModelRequest()); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not start")
	}
	cancel(cause)
	release()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled model succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("model did not join")
	}
	if _, err := state.outcome(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("local cancellation cause = %v", err)
	}
	if releases.Load() != 1 {
		t.Fatalf("captured model releases = %d", releases.Load())
	}
}
