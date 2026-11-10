package main

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/frame/sqlite"
)

func TestDirectoryAndAuthorizationSurviveRestartAndFenceWithdrawnGrants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connector.db")
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	open := func() catalog {
		db, err := sqlite.OpenSchema(t.Context(), path, []string{pluginstorage.CommandsSchema, entriesSchema, attemptsSchema})
		if err != nil {
			t.Fatal(err)
		}
		return catalog{db: db, now: func() time.Time { return now }}
	}
	c := open()
	name := "files"
	entry := wire.Entry{Name: name, LaunchGrant: "permitted-by-host", Enabled: true}
	change := func(expect string, replacement *wire.Entry, want wire.Decision) {
		t.Helper()
		result, err := invokeCatalogCommand(t, c, c.prepareChange, wire.Change{Name: name, Expected: expect, Replacement: replacement})
		if err != nil || result != want {
			t.Fatalf("change = %s, %v", result, err)
		}
	}
	current := func() wire.Entry {
		t.Helper()
		values, err := c.list(t.Context(), wire.Selector{Name: name})
		if err != nil || len(values.Entries) != 1 {
			t.Fatal(values, err)
		}
		return values.Entries[0]
	}
	begin := func(id, epoch string) wire.AttemptResult {
		t.Helper()
		a, err := invokeCatalogCommand(t, c, c.prepareStart, wire.Start{Name: name, Grant: entry.LaunchGrant, Epoch: epoch, AttemptID: id})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	read := func(id, epoch string) *wire.Attempt {
		t.Helper()
		r, err := c.get(t.Context(), wire.Request{ID: id, Epoch: epoch})
		if err != nil {
			t.Fatal(err)
		}
		return r.Attempt
	}
	change("", &entry, wire.Applied)
	change("", &entry, wire.Conflict)
	initial := current()
	begin("first", "host-A")
	begin("second", "host-A")
	if a := read("first", "host-A"); a == nil || a.Status != wire.Canceled {
		t.Fatal("superseded authorization remained live", a)
	}
	if err := c.db.Close(); err != nil {
		t.Fatal(err)
	}
	c = open()
	defer c.db.Close()
	if current() != initial {
		t.Fatal("directory changed at restart")
	}
	if result, err := invokeCatalogCommand(t, c, c.prepareRecovery, wire.Recovery{Epoch: "host-B"}); err != nil || result != wire.Applied {
		t.Fatal(result, err)
	}
	if a := read("second", "host-B"); a == nil || a.Status != wire.Unconfirmed {
		t.Fatal("lost authorization appeared active", a)
	}
	if result, err := invokeCatalogCommand(t, c, c.prepareFinish, wire.Completion{ID: "second", Epoch: "host-A", Status: wire.Succeeded}); err != nil || result != wire.Conflict {
		t.Fatal("late completion replaced cancellation", result, err)
	}
	if a := begin("third", "host-B"); a.Attempt == nil {
		t.Fatal("valid authorization unavailable")
	}
	entry.Enabled = false
	change(initial.Revision, &entry, wire.Applied)
	change(initial.Revision, nil, wire.Conflict)
	if a := read("third", "host-B"); a == nil || a.Status != wire.Canceled {
		t.Fatal(a)
	}
	if a := begin("fourth", "host-B"); a.Attempt != nil {
		t.Fatal("disabled server admitted authorization")
	}
	now = now.Add(wire.AttemptRetentionSeconds * time.Second)
	if a := read("third", "host-B"); a != nil {
		t.Fatal("expired attempt retained", a)
	}
	change(current().Revision, nil, wire.Applied)
}

func invokeCatalogCommand[P, R any](t *testing.T, c catalog, prepare func(context.Context, *sdk.ServerSession, P) (pluginstorage.CommandPlan[R], error), input P) (R, error) {
	t.Helper()
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	return pluginstorage.Command(t.Context(), c.db, "test", rand.Text(), admitted, nil, func(ctx context.Context, input P) (pluginstorage.CommandPlan[R], error) {
		return prepare(ctx, nil, input)
	})
}

func TestAuthorizationHistoryRejectsImpossibleStates(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "connector.db"), []string{attemptsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, test := range []struct {
		name     string
		status   wire.Outcome
		finished any
	}{
		{name: "pending_with_finish", status: wire.Pending, finished: int64(2)},
		{name: "settled_without_finish", status: wire.Succeeded},
		{name: "unknown_status", status: "invalid", finished: int64(2)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
				_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO attempts(id,name,epoch,status,started,finished) VALUES(?,?,?,?,?,?)`, test.name, "server", "epoch", test.status, int64(1), test.finished)
				return err
			})
			if err == nil {
				t.Fatal("accepted an impossible authorization state")
			}
		})
	}
}
