package mcpserver_test

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestOAuthTargetFingerprintAcceptsAdmittedHeaderBytes(t *testing.T) {
	name, err := mcpserver.ParseServerName("remote")
	if err != nil {
		t.Fatal(err)
	}
	server := mcpserver.Server{Name: name, Transport: mcpserver.TransportStreamableHTTP, Enabled: true, URL: "https://mcp.example.test/tools", Headers: map[string]string{"X-Opaque": string([]byte{0xff})}}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	target := mcpserver.OAuthTarget{Server: name, URL: server.URL, Headers: server.Headers}
	if !target.Matches(server) {
		t.Fatal("admitted server did not match its OAuth target")
	}
	fingerprint := target.Fingerprint()
	target.Headers = map[string]string{"X-Opaque": string([]byte{0xfe})}
	if fingerprint == target.Fingerprint() {
		t.Fatal("distinct credential bytes shared a target fingerprint")
	}
}

func TestOAuthTargetFingerprintPreservesConfigurationIdentity(t *testing.T) {
	name, err := mcpserver.ParseServerName("remote")
	if err != nil {
		t.Fatal(err)
	}
	base := mcpserver.OAuthTarget{Server: name, URL: "https://mcp.example.test/tools"}
	empty := base
	empty.Headers = map[string]string{}
	if !base.Equal(empty) || base.Fingerprint() != empty.Fingerprint() {
		t.Fatal("equal absent and empty header configurations have distinct identities")
	}
	first, second := base, base
	first.Headers = map[string]string{"X-A": "one", "X-B": "two"}
	second.Headers = map[string]string{"X-B": "two", "X-A": "one"}
	if !first.Equal(second) || first.Fingerprint() != second.Fingerprint() {
		t.Fatal("header insertion order changed target identity")
	}
	second.Headers = map[string]string{"X-A": "one:X-B", "X-B": "two"}
	if first.Fingerprint() == second.Fingerprint() {
		t.Fatal("header field boundaries changed without advancing target identity")
	}
}
