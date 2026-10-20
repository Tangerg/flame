package transport

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	"github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
	"github.com/Tangerg/flame/runtime/capability"
)

type observedRunHost struct {
	seen    chan schedule.RunRequest
	noticed chan struct{}

	db        *sql.DB
	namespace string
	calls     atomic.Int32
	lost      atomic.Bool
}

func (h *observedRunHost) Notify(context.Context, string) error {
	if h.noticed != nil {
		select {
		case h.noticed <- struct{}{}:
		default:
		}
	}
	return nil
}
func (h *observedRunHost) Namespace(context.Context) (string, error) { return h.namespace, nil }
func (h *observedRunHost) Admit(ctx context.Context, request schedule.RunRequest) error {
	// This read needs the database's single connection. Admission under an outer
	// private transaction would block it until this test's context expires.
	var rows int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM manual_firings`).Scan(&rows); err != nil {
		return err
	}
	if h.seen != nil {
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || invocation != request.Invocation() {
			return errors.New("original calling context changed")
		}
		h.seen <- request
	}
	h.calls.Add(1)
	if h.lost.Load() {
		return errors.New("Run acceptance observation lost")
	}
	return nil
}

func TestCapturedRuntimeTargetSurvivesRestartAndRefusesAnotherStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := bindRuntime(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := bindRuntime(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := bindRuntime(t.Context(), db, "another"); err == nil {
		t.Fatal("captured target was replaced")
	}
	var original string
	if err := db.QueryRow(`SELECT namespace FROM runtime_target WHERE id=1`).Scan(&original); err != nil || original != "original" {
		t.Fatalf("original target=%q error=%v", original, err)
	}
}

func schedulesContract(t *testing.T) *capability.Contract {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}
