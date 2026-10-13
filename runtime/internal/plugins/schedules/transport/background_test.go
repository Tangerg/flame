package transport

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/identity"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	"github.com/google/uuid"
)

func TestBackgroundOwnsCronCaptureAndJoinedLifetime(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewScheduleStore(db)
	definition, err := schedule.New("sch_due", schedule.Draft{Title: "Due", Instructions: "captured instructions", Cron: "@hourly", Enabled: true}, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(ctx, definition); err != nil {
		t.Fatal(err)
	}
	scanner, err := app.NewScanner(store, store, func() string { return identity.SessionPrefix + uuid.NewString() }, func() string { return identity.RunPrefix + uuid.NewString() })
	if err != nil {
		t.Fatal(err)
	}
	host := &observedRunHost{db: db, namespace: "owner", seen: make(chan schedule.RunRequest, 2), noticed: make(chan struct{}, 1)}
	var closes atomic.Int32
	target := capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}
	worker := newBackground(ctx, db, store, scanner, host, "schedules-provider", target, func() error { closes.Add(1); return nil })
	// Constructing provisional setup resources must not claim or start work.
	select {
	case <-host.seen:
		t.Fatal("setup started a Run")
	default:
	}
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	var captured schedule.RunRequest
	select {
	case captured = <-host.seen:
	case <-ctx.Done():
		t.Fatal("background capture did not start")
	}
	select {
	case <-host.noticed:
	case <-ctx.Done():
		t.Fatal("background changes were not published")
	}
	if captured.Invocation().Origin != capability.Automatic || captured.Invocation().Parent != "" || captured.Invocation().Target != target || captured.Execution().Snapshot().Instructions != definition.Execution().Snapshot().Instructions {
		t.Fatal("background capture changed caller or execution")
	}
	current, err := store.Get(ctx, definition.ID())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision() != definition.Revision()+2 || !current.NextRunAt().After(time.Now().Add(-time.Second)) {
		t.Fatalf("private claim/acceptance did not advance once: %+v", current.Snapshot())
	}
	if err := worker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := worker.Close(); err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatalf("activation connection closes=%d", closes.Load())
	}
	if err := worker.Start(); err == nil {
		t.Fatal("closed background restarted")
	}
	if pending, err := store.Pending(context.Background(), time.Time{}, "", 10); err != nil || len(pending) != 0 {
		t.Fatalf("accepted cron remained pending: %v, %v", pending, err)
	}
}
