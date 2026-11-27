package terminal

import (
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTerminalMCPFormPreservesOnlyExplicitStringInputs(t *testing.T) {
	connection, session := openSessionNavigationRuntime(t)
	host := openSessionNavigationTerminal(t, connection, connection, session)
	press := func(key input.Code) {
		t.Helper()
		if !host.Press(key) {
			t.Fatalf("terminal refused key %v", key)
		}
	}
	typeText := func(text string) {
		t.Helper()
		if !host.Type(text) {
			t.Fatal("terminal refused form input")
		}
	}
	replaceText := func(text string) {
		t.Helper()
		if !host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt}) {
			t.Fatal("terminal refused to select the field's text")
		}
		typeText(text)
	}

	const name = "argument-evidence"
	typeText("/mcp-create")
	press(input.Enter)
	host.Shows(t, "Create MCP server · 1/3")
	typeText(name)
	press(input.Tab)
	press(input.Down)
	press(input.Tab)
	press(input.Tab)
	press(input.Down)
	press(input.Enter)
	host.Shows(t, "Create MCP server · 2/3")

	typeText("/usr/bin/true")
	press(input.Tab)
	typeText(`["", "  ", null]`)
	press(input.Enter)
	host.Shows(t, "must be a JSON string")
	replaceText(`["", "  "]`)
	press(input.Tab)
	press(input.Down)
	press(input.Tab)
	typeText(`{"EMPTY":null}`)
	press(input.Enter)
	host.Shows(t, "within \"/EMPTY\": must be a JSON string")
	replaceText(`{"EMPTY":"","SPACE":"  "}`)
	press(input.Enter)
	host.Shows(t, "Create MCP server · 3/3")
	press(input.Enter)
	host.Shows(t, name+" · disabled")
	readConfigured := func() protocol.MCPServer {
		t.Helper()
		servers, err := connection.Servers(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(servers, func(server protocol.MCPServer) bool {
			return server.ID.Name == name && server.ID.Origin.Type == protocol.MCPOriginUser
		})
		if index < 0 {
			t.Fatalf("Runtime has no user server %q", name)
		}
		return servers[index]
	}
	created := readConfigured()
	empty, found := created.Connection.EnvMasked["EMPTY"]
	if created.Status.Type != protocol.MCPServerDisabled ||
		!slices.Equal(created.Connection.Args, []string{"", "  "}) ||
		!found || empty != "" || created.Connection.EnvMasked["SPACE"] == "" {
		t.Fatalf("configured server lost explicitly typed values: %+v", created)
	}

	press(input.Esc)
	host.Shows(t, "Ask flame")
	typeText("/mcp-edit " + name)
	press(input.Enter)
	host.Shows(t, "Configure MCP server · "+name+" · 1/2")
	press(input.Enter)
	host.Shows(t, "Configure MCP server · "+name+" · 2/2")
	const timeoutSeconds = 7
	typeText(strconv.Itoa(timeoutSeconds))
	press(input.Enter)
	host.Shows(t, "handshake timeout  "+strconv.Itoa(timeoutSeconds)+"s")
	updated := readConfigured()
	if updated.Revision == created.Revision || updated.HandshakeTimeout.Type != protocol.MCPHandshakeBounded ||
		*updated.HandshakeTimeout.Seconds != timeoutSeconds ||
		!slices.Equal(updated.Connection.Args, created.Connection.Args) ||
		!maps.Equal(updated.Connection.EnvMasked, created.Connection.EnvMasked) {
		t.Fatalf("timeout edit changed the original connection or failed to commit: %+v", updated)
	}
}
