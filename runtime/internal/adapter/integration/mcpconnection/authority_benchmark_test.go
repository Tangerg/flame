package mcpconnection

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type idleTool struct{}

func (idleTool) Definition() chat.ToolDefinition { return chat.ToolDefinition{Name: "idle"} }
func (idleTool) Call(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
	return chat.ToolOutput{}, nil
}

// BenchmarkAuthorizedToolCall measures the per-call source authorization of one
// tool of an installation that declares several stdio servers.
func BenchmarkAuthorizedToolCall(b *testing.B) {
	const servers = 8
	source := b.TempDir()
	var declared []string
	files := map[string]string{}
	for index := range servers {
		declared = append(declared, fmt.Sprintf(`"server%d":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/server%d/run.sh"],"cwd":"./server%d"}`, index, index, index))
		files[fmt.Sprintf("server%d/run.sh", index)] = "exit 0"
	}
	files["plugin.json"] = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"bench.authority"}`
	files["mcp.json"] = `{"$schema":"` + pluginpackage.MCPSchema + `","mcpServers":{` + strings.Join(declared, ",") + `}}`
	for index := range 4 {
		files[fmt.Sprintf("data-%d.bin", index)] = strings.Repeat("x", 1<<20)
	}
	for name, content := range files {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
	}
	db, err := sqlite.Open(b.Context(), filepath.Join(b.TempDir(), "flame.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	directory := filepath.Join(b.TempDir(), "plugins", "releases")
	b.Cleanup(func() {
		_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	catalog := sqlite.NewReleaseStore(db)
	releases, err := pluginpackage.New(directory, catalog)
	if err != nil {
		b.Fatal(err)
	}
	release, err := publishPackage(b.Context(), releases, source)
	if err != nil {
		b.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(b), source, release)
	if err != nil {
		b.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		b.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		b.Fatal(err)
	}
	installations := sqlite.NewInstallationStore(db)
	if err := installations.Save(b.Context(), installation); err != nil {
		b.Fatal(err)
	}
	if err := releases.Prepare(b.Context(), installation, release); err != nil {
		b.Fatal(err)
	}
	registry, err := plugins.NewRegistry(sqlite.NewMCPServerStore(db), installations, releases)
	if err != nil {
		b.Fatal(err)
	}
	id, err := installation.ServerID(testsupport.ServerName("server3"))
	if err != nil {
		b.Fatal(err)
	}
	server, _, err := registry.Dispatchable(b.Context(), id)
	if err != nil {
		b.Fatal(err)
	}
	config, err := configFromServer(server)
	if err != nil {
		b.Fatal(err)
	}
	authorized := authorizedTool{Tool: idleTool{}, registry: registry, config: config}
	for b.Loop() {
		if _, err := authorized.Call(b.Context(), toolcontract.Invocation{}); err != nil {
			b.Fatal(err)
		}
	}
}

func publishPackage(ctx context.Context, r *pluginpackage.Releases, source string) (plugin.Release, error) {
	candidate, err := r.Materialize(ctx, source)
	if err != nil {
		return plugin.Release{}, err
	}
	release, err := candidate.Publish(ctx)
	return release, errors.Join(err, candidate.Discard())
}
