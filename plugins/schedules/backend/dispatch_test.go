package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	framesqlite "github.com/Tangerg/frame/sqlite"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
)

type observedRunHost struct {
	seen    chan schedule.RunRequest
	noticed chan struct{}

	db        *framesqlite.Database
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
	if err := scanDatabase(ctx, h.db, `SELECT count(*) FROM manual_firings`, nil, &rows); err != nil {
		return err
	}
	if h.seen != nil {
		invocation, err := pluginmcp.InvocationFromContext(ctx)
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
	if err := scanDatabase(t.Context(), db, `SELECT namespace FROM runtime_target WHERE id=1`, nil, &original); err != nil || original != "original" {
		t.Fatalf("original target=%q error=%v", original, err)
	}
}

func schedulesContract(t *testing.T) *plugincontract.Capability {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}
