package pluginservices

import (
	"context"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type endpointResolver struct{ model chat.Model }

func (r endpointResolver) ResolveChat(context.Context, modelref.Selection) (models.ResolvedChat, error) {
	return models.NewResolvedChat(r.model, nil)
}

func TestEndpointEvidenceSurvivesOuterObserverFailureAndHostRestart(t *testing.T) {
	observers, bindings, _ := installedObserverFixture(t, "outer-failure")
	path := filepath.Join(t.TempDir(), "endpoint.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	message := chat.NewAssistantMessage(chat.NewTextPart("durable inner"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-model", Usage: &chat.Usage{InputTokens: 11, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	resolver, err := models.NewObservedResolver(endpointResolver{model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) { calls.Add(1); return response, nil })}, sqlite.NewModelInvocationStore(db))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.ResolveChat(t.Context(), testsupport.MustModelSelection("anthropic", "configured-model"))
	if err != nil {
		t.Fatal(err)
	}
	bound, _ := observers.Bind(resolved.Model(), nil, bindings)
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-endpoint-parent", Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "original-endpoint-parent"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	actual, err := bound.Call(ctx, request)
	if err == nil || !reflect.DeepEqual(actual, response) || calls.Load() != 1 {
		t.Fatalf("outer failure lost inner outcome: %+v, %d, %v", actual, calls.Load(), err)
	}
	if _, err := bound.Call(ctx, request); err == nil || calls.Load() != 1 {
		t.Fatalf("same calling operation replayed provider: %d, %v", calls.Load(), err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewModelInvocationStore(db)
	rows, err := store.PageEndpointInvocations(t.Context(), "original-endpoint-parent", 0, "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("persisted inner evidence: %+v, %v", rows, err)
	}
	row := rows[0]
	storedResponse, err := jsoninput.Encode(row.Response)
	if err != nil {
		t.Fatal(err)
	}
	expectedResponse, err := jsoninput.Encode(response)
	if err != nil {
		t.Fatal(err)
	}
	storedRequest, err := jsoninput.Encode(row.Invocation.Request)
	if err != nil {
		t.Fatal(err)
	}
	expectedRequest, err := jsoninput.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if row.State != "completed" || row.Failure != "" || storedResponse != expectedResponse || storedRequest != expectedRequest || row.Invocation.Provider != "anthropic" || row.Invocation.Model != "configured-model" || row.Invocation.Caller.Parent != "original-effect" || row.Invocation.ID == row.Invocation.Caller.ID {
		t.Fatalf("outer failure changed physical endpoint evidence: %+v", row)
	}
}
