package mcpserver_test

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func TestOAuthTargetFingerprintAcceptsAdmittedHeaderBytes(t *testing.T) {
	name, err := mcpserver.ParseServerName("remote")
	if err != nil {
		t.Fatal(err)
	}
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Transport: mcpserver.TransportStreamableHTTP, Enabled: true, URL: "https://mcp.example.test/tools", Headers: map[string]string{"X-Opaque": string([]byte{0xff})}}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	target := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: name, URL: server.URL, Headers: server.Headers}
	if target.Fingerprint() != server.OAuthTarget().Fingerprint() {
		t.Fatal("admitted server did not project its OAuth target")
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
	base := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: name, URL: "https://mcp.example.test/tools"}
	empty := base
	empty.Headers = map[string]string{}
	if base.Fingerprint() != empty.Fingerprint() {
		t.Fatal("equal absent and empty header configurations have distinct identities")
	}
	first, second := base, base
	first.Headers = map[string]string{"X-A": "one", "X-B": "two"}
	second.Headers = map[string]string{"X-B": "two", "X-A": "one"}
	if first.Fingerprint() != second.Fingerprint() {
		t.Fatal("header insertion order changed target identity")
	}
	second.Headers = map[string]string{"X-A": "one:X-B", "X-B": "two"}
	if first.Fingerprint() == second.Fingerprint() {
		t.Fatal("header field boundaries changed without advancing target identity")
	}
}

func TestInstallationOAuthTargetFollowsItsRecipient(t *testing.T) {
	name, err := mcpserver.ParseServerName("remote")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := resourceid.ParseInstallation("8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746")
	if err != nil {
		t.Fatal(err)
	}
	target := func(release, authority, recipient string, headers map[string]string) mcpserver.OAuthTarget {
		source, err := mcpserver.InstallationSource(installation, fingerprint.Strings(release), fingerprint.Strings(authority), fingerprint.Strings(recipient))
		if err != nil {
			t.Fatal(err)
		}
		return mcpserver.OAuthTarget{Source: source, Name: name, URL: "https://mcp.example.test/tools", Headers: headers}
	}
	base := target("release 1", "authority 1", "endpoint", map[string]string{"X-Tenant": "one"})
	if base.Fingerprint() != target("release 2", "authority 2", "endpoint", map[string]string{"X-Tenant": "rotated"}).Fingerprint() {
		t.Fatal("an unchanged recipient lost its credential across a release or input change")
	}
	if base.Fingerprint() == target("release 1", "authority 1", "changed endpoint", nil).Fingerprint() {
		t.Fatal("a changed recipient kept the credential")
	}
}
