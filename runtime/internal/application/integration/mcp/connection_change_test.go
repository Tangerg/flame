package mcp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type countedConnections struct {
	*fakePorts
	begins atomic.Int32
}

func (c *countedConnections) BeginConnect(ctx context.Context, name mcpserver.ID) (Attempt, error) {
	c.begins.Add(1)
	return c.fakePorts.BeginConnect(ctx, name)
}

func TestServerUpdateUsesTheDomainConnectionIdentity(t *testing.T) {
	bounded, err := mcpserver.NewHandshakeTimeout(17 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	description := "new notes"
	enabled := true
	for _, test := range []struct {
		name             string
		initiallyEnabled bool
		patch            ServerPatch
		wantConnect      bool
	}{
		{name: "description", initiallyEnabled: true, patch: ServerPatch{Description: &description}},
		{name: "future handshake timeout", initiallyEnabled: true, patch: ServerPatch{HandshakeTimeout: &bounded}},
		{name: "same enablement", initiallyEnabled: true, patch: ServerPatch{Enabled: &enabled}},
		{name: "identical connection", initiallyEnabled: true, patch: ServerPatch{Connection: &ConnectionInput{Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/current"}}},
		{name: "changed endpoint", initiallyEnabled: true, wantConnect: true, patch: ServerPatch{Connection: &ConnectionInput{Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/changed"}}},
		{name: "enable", wantConnect: true, patch: ServerPatch{Enabled: &enabled}},
		{name: "disabled description", patch: ServerPatch{Description: &description}},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := testsupport.UserMCPServer("files")
			current := mcpserver.Server{Source: mcpserver.UserSource(), Name: name.Name(), Enabled: test.initiallyEnabled, Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/current"}
			registry := &testRegistry{servers: map[mcpserver.ID]mcpserver.Server{name: current}}
			ports := &countedConnections{fakePorts: &fakePorts{statuses: []mcpserver.ConnectionStatus{{Server: name, State: mcpserver.ConnectionConnected, ToolCount: 3}}}}
			coordinator := testCoordinator(t, Config{Registry: registry, Connections: ports, ToolCatalog: ports})
			updated, err := coordinator.UpdateServer(t.Context(), name, test.patch)
			if err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if test.wantConnect {
				want = 1
			}
			if got := ports.begins.Load(); got != want {
				t.Fatalf("connection admissions = %d, want %d", got, want)
			}
			stored, found, err := registry.Get(t.Context(), name)
			if err != nil || !found {
				t.Fatalf("stored update = %t, %v", found, err)
			}
			if test.patch.Description != nil && (stored.Description != description || updated.Description != description) {
				t.Fatalf("notes were not committed: %+v, %+v", stored, updated)
			}
			if test.patch.HandshakeTimeout != nil && stored.HandshakeTimeout != bounded {
				t.Fatal("future handshake policy was not committed")
			}
			if current.Enabled && !test.wantConnect && (updated.State.Type != ServerConnected || updated.State.ToolCount == nil || *updated.State.ToolCount != 3) {
				t.Fatalf("metadata update withdrew the live connection: %+v", updated.State)
			}
		})
	}
}
