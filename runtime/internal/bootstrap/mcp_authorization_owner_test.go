package bootstrap

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMCPAuthorizationReadsTheOriginalConnector(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	original := installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: "original-connector-auth", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: poMCPRemote(t, "record")},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-original-server")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })

	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	const key = "start-authorization-on-original"
	ref := r.must(delivery.MCPAuthorizationAttemptsCreate, request, key).(*protocol.MCPAuthorizationAttemptRef)
	accepted := awaitMCPAuthorization(t, r, ref.AttemptID)
	if accepted.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("the original connector did not accept the result: %+v", accepted)
	}

	replacement := r.must(delivery.PluginsInstall,
		protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "mcp")}, "install-replacement-connector").(*protocol.PluginInstallation)
	if replacement.ID == original.ID {
		t.Fatal("replacement must have a separate private directory")
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: replacement.ID, Enabled: true}, "enable-replacement-connector")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{
		Role: mcpconnector.Reference, InstallationID: replacement.ID, Contribution: mcpconnector.Export,
	}, "select-replacement-connector")

	replayed := r.must(delivery.MCPAuthorizationAttemptsCreate, request, key).(*protocol.MCPAuthorizationAttemptRef)
	if *replayed != *ref {
		t.Fatalf("selection changed the accepted reference: %+v, want %+v", replayed, ref)
	}
	observed, err := r.api.GetMCPAuthorizationAttempt(t.Context(), ref.AttemptID)
	if err != nil || !reflect.DeepEqual(observed, accepted) {
		t.Fatalf("selection changed the owned outcome: %+v, %v; want %+v", observed, err, accepted)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	observed, err = r.api.GetMCPAuthorizationAttempt(t.Context(), ref.AttemptID)
	if err != nil || !reflect.DeepEqual(observed, accepted) {
		t.Fatalf("restart lost the original connector route: %+v, %v; want %+v", observed, err, accepted)
	}

	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: original.ID, Enabled: false}, "disable-original-connector")
	if _, err := r.api.GetMCPAuthorizationAttempt(t.Context(), ref.AttemptID); !errors.Is(err, protocol.ErrPluginUnavailable) {
		t.Fatalf("revoked owner must report unavailability: %v", err)
	}
}

func awaitMCPAuthorization(t *testing.T, r *poMCPRuntime, id string) *protocol.MCPAuthorizationAttempt {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		attempt, err := r.api.GetMCPAuthorizationAttempt(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if attempt.Status.Type != protocol.MCPAuthorizationAttemptPending {
			return attempt
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authorization did not settle")
	return nil
}
