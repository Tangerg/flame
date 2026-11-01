package main

import (
	"path/filepath"
	"testing"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/qimu/sqlite"
)

func TestDirectoryAndAuthorizationSurviveRestartAndFenceWithdrawnGrants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connector.db")
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	open := func() catalog {
		db, err := sqlite.OpenSchema(t.Context(), path, []string{entriesSchema, attemptsSchema})
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
		result, err := c.change(t.Context(), nil, wire.Change{Name: name, Expected: expect, Replacement: replacement})
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
		a, err := c.start(t.Context(), nil, wire.Start{Name: name, Grant: entry.LaunchGrant, Epoch: epoch, AttemptID: id})
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
	if result, err := c.recover(t.Context(), nil, wire.Recovery{Epoch: "host-B"}); err != nil || result != wire.Applied {
		t.Fatal(result, err)
	}
	if a := read("second", "host-B"); a == nil || a.Status != wire.Unconfirmed {
		t.Fatal("lost authorization appeared active", a)
	}
	if result, err := c.finish(t.Context(), nil, wire.Completion{ID: "second", Epoch: "host-A", Status: wire.Succeeded}); err != nil || result != wire.Conflict {
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
