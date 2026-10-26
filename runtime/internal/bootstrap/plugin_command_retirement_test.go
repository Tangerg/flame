package bootstrap

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestPreparedInstalledCommandRetiresAfterRestartWithoutBackendDispatch(t *testing.T) {
	source := reviewExamplePackage(t)
	method := "io.github.tangerg.flame.examples/reviews.update"
	evidence := filepath.Join(t.TempDir(), "dispatched.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "request-probe"), "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build request probe: %s, %v", output, err)
	}
	manifest, err := json.Marshal(map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{"reviews": map[string]any{"type": "stdio", "command": "./bin/request-probe", "args": []string{evidence, "./bin/reviews", method, "", "request"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	directory, err := localruntime.DataDirectoryAt(cfg.stores.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A refused dispatch write leaves the same durable prepared route as a caller
	// canceled between binding and admission, without adding a production hook.
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_dispatch BEFORE UPDATE OF phase ON plugin_commands WHEN NEW.phase='dispatched' BEGIN SELECT RAISE(ABORT,'dispatch storage refused'); END`); err != nil {
		t.Fatal(err)
	}
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: contribution.ID, Contract: contribution.Contract, Method: method, Input: `{"id":"example-review","expectedRevision":1,"status":"resolved"}`, Target: extension.Target{Scope: "installation", Reference: installed.ID}}
	options := delivery.Options{IdempotencyKey: "prepared-review"}
	ctx, cancel := context.WithCancel(t.Context())
	result := r.endpoint.Invoke(ctx, delivery.PluginsCall, request, options)
	cancel()
	if result.Failure == nil {
		t.Fatal("refused dispatch appeared successful")
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER refuse_dispatch`); err != nil {
		t.Fatal(err)
	}
	id := operation.Identity(operation.Bind(t.Context(), cfg.stores.IdempotencyNamespace.String(), options.IdempotencyKey))
	command, found, err := cfg.stores.PluginCommands.Get(t.Context(), id)
	if err != nil || !found || command.Snapshot().Phase != plugin.CommandPrepared {
		t.Fatalf("prepared route=%v, %t, %v", command, found, err)
	}
	installed = stageCommandReplacement(t, r, installed, source)
	assertUnresolvedInstallationIsHeld(t, r, installed)
	r = reopenCommandRuntime(t, r, &cfg)
	assertUnresolvedInstallationIsHeld(t, r, installed)
	lookup := protocol.PluginCommandRequest{Namespace: cfg.stores.IdempotencyNamespace.String(), Key: options.IdempotencyKey}
	wrongStore := lookup
	wrongStore.Namespace = "idp_00000000000000000000000000000000"
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsRetireCommand, wrongStore, delivery.Options{}); !errors.Is(result.Failure, protocol.ErrIdempotencyStoreMismatch) {
		t.Fatalf("retirement crossed stores: %+v", result)
	}
	for range 2 {
		r.must(delivery.PluginsRetireCommand, lookup, "")
	}
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsGetCommand, lookup, delivery.Options{}); !errors.Is(result.Failure, protocol.ErrPluginCommandRetired) {
		t.Fatalf("retirement not publicly observable: %+v", result)
	}
	r.must(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Staged.Digest}, "select-after-retirement")
	r = reopenCommandRuntime(t, r, &cfg)
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options); !errors.Is(result.Failure, protocol.ErrPluginCommandRetired) {
		t.Fatalf("retired command was replayed: %+v", result)
	}
	if _, err := os.Stat(evidence); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired command reached backend: %v", err)
	}
}
