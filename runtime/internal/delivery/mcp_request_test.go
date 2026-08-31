package server

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMCPServerInputRejectsContradictoryToolPolicy(t *testing.T) {
	_, err := mcpServerInputFromCandidate(protocol.MCPServerCandidate{
		Name:    "files",
		Enabled: true,
		Connection: protocol.MCPConnectionInput{
			Type:    protocol.MCPTransportStdio,
			Command: "mcp-files",
		},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		DisabledTools:    []string{"read"},
		AutoApproveTools: []string{"read"},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("mcpServerInputFromCandidate error = %v, want ErrInvalidParams", err)
	}
}

func TestMCPServerPatchRejectsInvalidRemoteToolIdentity(t *testing.T) {
	invalid := []string{"tool/name"}
	_, err := mcpServerPatchFromRequest(protocol.UpdateMCPServerRequest{
		Server:        "files",
		DisabledTools: &invalid,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("mcpServerPatchFromRequest error = %v, want ErrInvalidParams", err)
	}
}
