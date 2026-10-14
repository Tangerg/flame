package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

func TestUnobservedEndpointReopensUnknownWithoutReplayingCallingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured input")))
	invocation := modelref.EndpointInvocation{ID: "endpoint-original", Caller: capability.Invocation{ID: "model-operation", Origin: capability.Human, Parent: "human-command", Target: capability.Target{Scope: "call", Reference: "model-operation"}}, Provider: "anthropic", Model: "captured-model", Request: request, StartedAt: time.Now().UTC()}
	if err := sqlite.NewModelInvocationStore(db).StartEndpointInvocation(t.Context(), invocation); err != nil {
		t.Fatal(err)
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
	if err := store.ReconcileEndpointInvocations(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rows, err := store.PageEndpointInvocations(t.Context(), "model-operation", 0, "", 10)
	if err != nil || len(rows) != 1 || rows[0].State != "unknown" || rows[0].Response != nil || rows[0].Failure == "" {
		t.Fatalf("restart promoted unknown request: %+v, %v", rows, err)
	}
	invocation.ID = "endpoint-replacement"
	if err := store.StartEndpointInvocation(t.Context(), invocation); err == nil {
		t.Fatal("same calling operation started a replacement endpoint")
	}
}
