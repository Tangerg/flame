package mcp

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestCommittedSourceRemovalRevokesExposureWithoutRegistryRereads(t *testing.T) {
	for _, remove := range []bool{false, true} {
		name := "disable"
		if remove {
			name = "delete"
		}
		t.Run(name, func(t *testing.T) {
			server := mcpserver.Server{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
			ref := testMCPRef(server.Name, testRemoteToolName("read"))
			registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{server.Name: server}, listErr: errors.New("catalog read unavailable")}
			exposure := NewExposureState([]mcpserver.Server{server}, nil)
			c := testCoordinator(t, Config{Registry: registry, Exposure: exposure})
			var err error
			if remove {
				err = c.DeleteServer(t.Context(), server.Name)
			} else {
				_, err = c.UpdateServer(t.Context(), server.Name, ServerPatch{Enabled: new(false)})
			}
			if !exposure.ToolDisabled(ref) {
				t.Error("committed source removal left tools exposed")
			}
			if err != nil {
				t.Fatalf("acknowledged write depended on a catalog reread: %v", err)
			}
		})
	}
}

func TestExposureRemovalSurvivesLiveDetachFailure(t *testing.T) {
	server := mcpserver.Server{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
	ref := testMCPRef(server.Name, testRemoteToolName("read"))
	exposure := NewExposureState([]mcpserver.Server{server}, nil)
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{server.Name: server}, listErr: errors.New("catalog read unavailable")}
	detachErr := errors.New("detach failed")
	c := testCoordinator(t, Config{Registry: registry, Exposure: exposure, ConnectionLifecycle: &fakePorts{removeErr: detachErr}})
	if err := c.DeleteServer(t.Context(), server.Name); !errors.Is(err, detachErr) {
		t.Fatalf("lost detach failure: %v", err)
	}
	if !exposure.ToolDisabled(ref) {
		t.Fatal("failed live detachment left a deleted source exposed")
	}
}

func TestExposureRetainsToolChoicesUntilSourceDeletion(t *testing.T) {
	server := mcpserver.Server{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
	ref := testMCPRef(server.Name, testRemoteToolName("read"))
	other := testMCPRef(testMCPServerName("other"), testRemoteToolName("read"))
	exposure := NewExposureState([]mcpserver.Server{server, {Name: other.Server(), Enabled: true}}, nil)
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{server.Name: server}, listErr: errors.New("catalog read unavailable")}
	c := testCoordinator(t, Config{Registry: registry, Exposure: exposure})
	if err := c.SetToolExposure(t.Context(), ref, true); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		if _, err := c.UpdateServer(t.Context(), server.Name, ServerPatch{Enabled: new(enabled)}); err != nil {
			t.Fatal(err)
		}
		if !exposure.ToolDisabled(ref) {
			t.Fatal("source enablement erased a per-tool choice")
		}
		if exposure.ToolDisabled(other) {
			t.Fatal("source enablement hid an unrelated source")
		}
	}
	if err := c.DeleteServer(t.Context(), server.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateServer(t.Context(), input(server)); err != nil {
		t.Fatal(err)
	}
	if exposure.ToolDisabled(ref) {
		t.Fatal("recreated source inherited deleted exposure")
	}
}
