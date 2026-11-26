package bootstrap

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMCPAuthorizationFinishesWithoutAnObserver(t *testing.T) {
	source, withheld := connectorWithLostFinish(t)
	remote := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: "authorization-finish", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-finish-server")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	ref := r.must(delivery.MCPAuthorizationAttemptsCreate, request, "begin-finish-without-observer").(*protocol.MCPAuthorizationAttemptRef)

	path := filepath.Join(cfg.stores.DataDirectory, "plugins", "data", installed.ID, "external-servers.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	awaitAuthorizationOutcome(t, db, "SELECT status FROM attempts WHERE id=?", ref.AttemptID, mcpconnector.Succeeded)
	if _, err := os.Stat(withheld); err != nil {
		t.Fatalf("the original finish request was not withheld: %v", err)
	}
}

func connectorWithLostFinish(t *testing.T) (source, evidence string) {
	t.Helper()
	methods, err := connectormcp.NewMethods()
	if err != nil {
		t.Fatal(err)
	}
	source = filepath.Join(t.TempDir(), "mcp")
	if err := os.CopyFS(source, os.DirFS(filepath.Join(firstPartyPackages, "mcp"))); err != nil {
		t.Fatal(err)
	}
	evidence = filepath.Join(t.TempDir(), "finish-request.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "message-loss"), "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build message-loss fixture: %v\n%s", err, output)
	}
	const schema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
	const connector = "./bin/connector"
	declaration := fmt.Sprintf(`{"$schema":%q,"mcpServers":{"connector":{"type":"stdio","command":"./bin/message-loss","args":[%q,%q,%q,"","request"]}}}`,
		schema, evidence, connector, methods.Finish.Name())
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), []byte(declaration), 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, source)
	return source, evidence
}
