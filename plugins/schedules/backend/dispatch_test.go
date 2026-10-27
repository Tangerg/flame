package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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
		invocation, err := qimumcp.InvocationFromContext(ctx)
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
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "original"); err != nil {
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
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "another"); err == nil {
		t.Fatal("captured target was replaced")
	}
	var original string
	if err := db.QueryRow(`SELECT namespace FROM runtime_target WHERE id=1`).Scan(&original); err != nil || original != "original" {
		t.Fatalf("original target=%q error=%v", original, err)
	}
}

func schedulesContract(t *testing.T) *portable.Capability {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}
