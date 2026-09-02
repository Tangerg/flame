package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"strings"
	"testing"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/models/catalog"
)

func constClient(c chatclient.Client) modeladapter.AuxiliaryResolver {
	return func(context.Context) (*chatclient.Client, error) { return &c, nil }
}

func unexpectedClient(context.Context) (*chatclient.Client, error) {
	return nil, errors.New("unexpected utility model call")
}

func textToolOutput(t *testing.T, output chat.ToolOutput) string {
	t.Helper()
	text, ok := output.Text()
	if !ok {
		t.Fatal("Tool output contains non-text content")
	}
	return text
}

func intPointer(value int) *int { return &value }

type recordingSessionContextInvalidator struct {
	sessions []string
}

func (r *recordingSessionContextInvalidator) ForgetSessionContext(sessionID string) {
	r.sessions = append(r.sessions, sessionID)
}

func mustNewCompactor(
	t *testing.T,
	store compactionStore,
	client modeladapter.AuxiliaryResolver,
	liveState LiveStateSnapshotter,
	values CompactionPolicyValues,
	contextStates ...SessionContextInvalidator,
) *Compactor {
	t.Helper()
	if len(contextStates) > 1 {
		t.Fatal("mustNewCompactor accepts at most one context invalidator")
	}
	var contextState SessionContextInvalidator
	if len(contextStates) == 1 {
		contextState = contextStates[0]
	}
	compactor, err := NewCompactor(store, client, liveState, values, contextState)
	if err != nil {
		t.Fatal(err)
	}
	return compactor
}

func mustCompactionPolicy(t *testing.T, values CompactionPolicyValues) compactionPolicy {
	t.Helper()
	policy, err := newCompactionPolicy(values)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

type compactionTestStore struct {
	*testsupport.ConversationStore
	rewrites int
}

func newCompactionTestStore() *compactionTestStore {
	return &compactionTestStore{ConversationStore: testsupport.NewConversationStore()}
}

func testTokenLimits(t *testing.T, values modelref.TokenLimitValues) modelref.TokenLimits {
	t.Helper()
	limits, err := modelref.NewTokenLimits(values)
	if err != nil {
		t.Fatal(err)
	}
	return limits
}

func testTokenLimit(value int64) *int64 { return &value }

func TestCompactionPolicyPreservesOptionalPresence(t *testing.T) {
	defaults, err := newCompactionPolicy(CompactionPolicyValues{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.maxTokensExplicit {
		t.Fatalf("default policy = %+v", defaults)
	}

	zero := 0
	for name, values := range map[string]CompactionPolicyValues{
		"tokens": {MaxTokens: &zero},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newCompactionPolicy(values); err == nil {
				t.Fatal("present zero was treated as an omitted policy value")
			}
		})
	}
}

func TestCompactorTokenTriggerDoesNotExceedCatalogInputLimit(t *testing.T) {
	model, found := catalog.Default.Lookup("openai", "gpt-5.4-mini")
	if !found {
		t.Fatal("catalog omitted openai/gpt-5.4-mini")
	}
	policy := mustCompactionPolicy(t, CompactionPolicyValues{})
	trigger, err := policy.tokenTrigger(
		testTokenLimits(t, modelref.TokenLimitValues{
			ContextWindow:   testTokenLimit(model.Limits.ContextWindow),
			MaxInputTokens:  testTokenLimit(model.Limits.MaxInputTokens),
			MaxOutputTokens: testTokenLimit(model.Limits.MaxOutputTokens),
		}),
		chat.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if trigger != int(model.Limits.MaxInputTokens) {
		t.Fatalf("token trigger = %d, want provider max input %d", trigger, model.Limits.MaxInputTokens)
	}
}

func TestCompactorTokenTriggerReservesExplicitOutputWindow(t *testing.T) {
	model, found := catalog.Default.Lookup("alibaba", "qwen-mt-plus")
	if !found {
		t.Fatal("catalog omitted alibaba/qwen-mt-plus")
	}
	if model.Limits.MaxInputTokens != 0 {
		t.Fatalf("fixture max input = %d, want unknown", model.Limits.MaxInputTokens)
	}
	policy := mustCompactionPolicy(t, CompactionPolicyValues{})
	requestedOutput := model.Limits.MaxOutputTokens
	trigger, err := policy.tokenTrigger(
		testTokenLimits(t, modelref.TokenLimitValues{
			ContextWindow:   testTokenLimit(model.Limits.ContextWindow),
			MaxOutputTokens: testTokenLimit(model.Limits.MaxOutputTokens),
		}),
		chat.Options{MaxOutputTokens: &requestedOutput},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := int(model.Limits.ContextWindow - model.Limits.MaxOutputTokens)
	if trigger != want {
		t.Fatalf("token trigger = %d, want %d after reserving explicit output window", trigger, want)
	}
}

func TestCompactorTokenTriggerKeepsLimitOwnershipExplicit(t *testing.T) {
	t.Run("explicit trigger cannot exceed provider input", func(t *testing.T) {
		policy := mustCompactionPolicy(t, CompactionPolicyValues{MaxTokens: intPointer(300_000)})
		trigger, err := policy.tokenTrigger(
			testTokenLimits(t, modelref.TokenLimitValues{
				ContextWindow: testTokenLimit(400_000), MaxInputTokens: testTokenLimit(272_000),
			}),
			chat.Options{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if trigger != 272_000 {
			t.Fatalf("token trigger = %d, want hard input cap 272000", trigger)
		}
	})

	t.Run("selected model does not inherit unrelated fallback input", func(t *testing.T) {
		policy := mustCompactionPolicy(t, CompactionPolicyValues{})
		trigger, err := policy.tokenTrigger(
			testTokenLimits(t, modelref.TokenLimitValues{ContextWindow: testTokenLimit(1_000_000)}),
			chat.Options{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if trigger != 800_000 {
			t.Fatalf("token trigger = %d, want selected model context trigger 800000", trigger)
		}
	})

	t.Run("window percentage cannot overflow", func(t *testing.T) {
		policy := mustCompactionPolicy(t, CompactionPolicyValues{})
		trigger, err := policy.tokenTrigger(
			testTokenLimits(t, modelref.TokenLimitValues{ContextWindow: testTokenLimit(int64(math.MaxInt))}),
			chat.Options{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if trigger <= 0 {
			t.Fatalf("token trigger overflowed to %d", trigger)
		}
	})
}

func (c *compactionTestStore) RewriteForCompaction(
	ctx context.Context,
	sessionID string,
	expectedCount int,
	_, _ int,
	messages ...chat.Message,
) error {
	current, err := c.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(current) != expectedCount {
		return fmt.Errorf("test compaction count changed from %d to %d", expectedCount, len(current))
	}
	if err := c.Replace(ctx, sessionID, messages...); err != nil {
		return err
	}
	c.rewrites++
	return nil
}

func TestTrimForBudgetPreviewsOldNotRecentAndDoesNotMutate(t *testing.T) {
	bigArgs := strings.Repeat("a", 5_000)
	bigResult := strings.Repeat("b", 5_000)
	msgs := []chat.Message{
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "write", Arguments: bigArgs})),
		chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "write", Output: chat.NewTextToolOutput(bigResult)}),
		chat.NewToolMessage(chat.ToolResult{ID: "c2", Name: "read", Output: chat.NewTextToolOutput(bigResult)}),
		chat.NewAssistantMessage(chat.NewTextPart("x")),
	}
	trimmed, changed := trimForBudgetBefore(msgs, 2)
	if !changed {
		t.Fatal("expected the old oversized parts to be trimmed")
	}
	gotArgs := trimmed[0].Parts[0].ToolCall.Arguments
	if len(gotArgs) >= len(bigArgs) || !strings.Contains(gotArgs, "_trimmed") {
		t.Fatalf("args not trimmed: %q", gotArgs)
	}
	if !json.Valid([]byte(gotArgs)) {
		t.Fatalf("trimmed args must stay valid JSON, got %q", gotArgs)
	}
	if got := textToolOutput(t, trimmed[1].Parts[0].ToolResult.Output); len(got) >= len(bigResult) || !strings.Contains(got, "trimmed on compaction") {
		t.Fatalf("old result not previewed: len %d", len(got))
	}
	if got := textToolOutput(t, trimmed[2].Parts[0].ToolResult.Output); got != bigResult {
		t.Fatal("recent result must be left full")
	}
	if msgs[0].Parts[0].ToolCall.Arguments != bigArgs || textToolOutput(t, msgs[1].Parts[0].ToolResult.Output) != bigResult {
		t.Fatal("trimForBudget mutated its input's shared parts")
	}
}

type textStubModel struct {
	reply    string
	calls    int
	requests []*chat.Request
}

func newTextStubModel(reply string) *textStubModel {
	return &textStubModel{reply: reply}
}

func (t *textStubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	t.calls++
	t.requests = append(t.requests, request)
	message := chat.NewAssistantMessage(chat.NewTextPart(t.reply))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (t *textStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(t.Call(ctx, req))
}
