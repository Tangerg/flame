package main

import (
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/plugins/memory/internal/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func connectMemoryBackend(t *testing.T, path string) (*sqlite.AgentMemoryStore, *sdk.ClientSession, func()) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewAgentMemoryStore(db)
	server := sdk.NewServer(&sdk.Implementation{Name: "memory"}, nil)
	if err := registerServer(server, db, store, filesystem.DirectoryResolver{}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeBackend := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), backend.Close(), db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(closeBackend)
	return store, session, closeBackend
}
