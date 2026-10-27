package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/flame/runtime/internal/testsupport/pluginhost"
)

// BenchmarkInstalledSourceAdmission measures the source verification that each
// tool call performs, independently of transport latency or tool execution.
func BenchmarkInstalledSourceAdmission(b *testing.B) {
	const servers = 8
	source := b.TempDir()
	var declared []string
	files := map[string]string{}
	for index := range servers {
		declared = append(declared, fmt.Sprintf(`"server%d":{"type":"stdio","command":"./server%d/run","cwd":"./server%d"}`, index, index, index))
		files[fmt.Sprintf("server%d/run", index)] = "#!/bin/sh\nexit 0\n"
	}
	files["plugin.json"] = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"bench.authority","version":"1.0.0"}`
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
	if _, err := pluginpackage.BuildRelease(b.Context(), source, pluginpackage.BuildTarget{OS: runtime.GOOS, Arch: runtime.GOARCH}, io.Discard); err != nil {
		b.Fatal(err)
	}
	db, err := persistencefixture.Open(b, b.Context(), filepath.Dir(filepath.Join(b.TempDir(), "flame.db")))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	directory := filepath.Join(b.TempDir(), "plugins")
	content, err := distribution.Open(distribution.Config{
		Directory: filepath.Join(directory, "releases"), Admission: db.Admission,
		Trust:    func(context.Context, distribution.Source, distribution.Manifest, distribution.Lock) error { return nil },
		Retained: func(context.Context, qimu.Digest) (bool, error) { return true, nil },
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = content.Close(context.Background()) })
	catalog := db.PluginReleases
	releases, err := pluginpackage.New(content, filepath.Join(directory, "data"), catalog)
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
	if err := installation.Enable(release); err != nil {
		b.Fatal(err)
	}
	installations := db.Installations
	framework := pluginhost.New(b, db.Bundle)
	if err := framework.Apply(b.Context(), installation); err != nil {
		b.Fatal(err)
	}
	if err := releases.Prepare(b.Context(), installation, release); err != nil {
		b.Fatal(err)
	}
	registry, err := plugins.NewRegistry(framework.Host, db.MCPServers, installations, releases, db.ProviderSelections, db.PluginCompositions)
	if err != nil {
		b.Fatal(err)
	}
	id, err := installation.ServerID(testsupport.ServerName("server3"))
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if server, found, err := registry.Dispatchable(b.Context(), id); err != nil || !found || !server.Enabled {
			b.Fatalf("source admission: found=%v enabled=%v error=%v", found, server.Enabled, err)
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
