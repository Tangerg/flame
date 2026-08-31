package session

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

func TestOpenCreatesOrRestoresAValidatedSnapshot(t *testing.T) {
	runtime := runtimefixture.New()

	created, err := Open(t.Context(), runtime, "", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Session.ID == "" || len(created.Transcript) != 0 || len(created.Runs) != 0 {
		t.Fatalf("created snapshot = %+v", created)
	}

	restored, err := Open(t.Context(), runtime, created.Session.ID, "ignored")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Session != created.Session {
		t.Fatalf("restored session = %+v, want %+v", restored.Session, created.Session)
	}
}

func TestOpenPreservesRuntimeErrorIdentity(t *testing.T) {
	_, err := Open(t.Context(), runtimefixture.New(), "missing", "")
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("open error = %v, want session-not-found identity", err)
	}
}
