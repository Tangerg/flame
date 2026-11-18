package pluginpackage

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/frame/distribution"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

// Fixtures explicitly include the neutral author inputs. The original product
// schema remains independent, so malformed metadata still reaches its parser.
func writePackage(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for name, body := range files {
		if name == "plugin.json" {
			var object map[string]jsontext.Value
			if json.Unmarshal([]byte(body), &object) == nil && object != nil {
				if _, exists := object["version"]; !exists {
					object["version"] = jsontext.Value(`"1.0.0"`)
					encoded, _ := json.Marshal(object)
					body = string(encoded)
				}
			}
		}
		destination := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mcp struct {
		Servers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal([]byte(files["mcp.json"]), &mcp) == nil {
		for _, server := range mcp.Servers {
			command, ok := strings.CutPrefix(server.Command, "./")
			if !ok || !plugin.ValidResourcePath(command) {
				continue
			}
			if _, exists := files[command]; !exists {
				continue
			}
			if err := os.Chmod(filepath.Join(directory, command), 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Handwritten module fixtures have a matching explicit compiler graph. Real
	// generated graphs are tested by component-sdk with its actual compiler.
	var declaration struct {
		Components []struct {
			Entry  string `json:"entry"`
			Export string `json:"export"`
		} `json:"components"`
	}
	for name, body := range files {
		if !strings.HasSuffix(name, "declaration.json") {
			continue
		}
		if json.Unmarshal([]byte(body), &declaration) != nil {
			continue
		}
		graphs := map[string]*moduleGraph{}
		for _, component := range declaration.Components {
			if !plugin.ValidResourcePath(component.Entry) || component.Export == "" {
				continue
			}
			graphPath := path.Join(path.Dir(component.Entry), moduleGraphFilename)
			graph := graphs[graphPath]
			if graph == nil {
				graph = &moduleGraph{SchemaVersion: moduleGraphSchemaVersion, Entry: path.Base(component.Entry), Modules: []moduleEntry{{Path: path.Base(component.Entry), Imports: []string{}, DynamicImports: []string{}, Exports: []string{}}}, SharedSpecifiers: []string{}}
				graphs[graphPath] = graph
			}
			graph.Modules[0].Exports = append(graph.Modules[0].Exports, component.Export)
		}
		for name, graph := range graphs {
			if _, exists := files[name]; exists {
				continue
			}
			encoded, _ := json.Marshal(graph)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(directory, name)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, name), encoded, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return directory
}

func testReleases(t *testing.T) *Releases { releases, _, _ := testReleaseStore(t); return releases }

func publishPackage(ctx context.Context, releases *Releases, source string) (plugin.Release, error) {
	info, err := os.Stat(source)
	if err != nil {
		return plugin.Release{}, err
	}
	if info.IsDir() {
		if _, err := BuildRelease(ctx, source, BuildTarget{OS: runtime.GOOS, Arch: runtime.GOARCH}, io.Discard); err != nil {
			return plugin.Release{}, err
		}
	}
	candidate, err := releases.Materialize(ctx, source)
	if err != nil {
		return plugin.Release{}, err
	}
	release, err := candidate.Publish(ctx)
	return release, errors.Join(err, candidate.Discard())
}

func testReleaseStore(t *testing.T) (*Releases, *installationFixture, *persistence.MCPServerStore) {
	t.Helper()
	directory := t.TempDir()
	db, err := persistencefixture.Open(t, t.Context(), filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := framehost.New(t.Context(), framehost.Config{Authority: db.Frame, Drivers: []framehost.Driver{unopenedProfile(pluginmcp.ProcessProfile), unopenedProfile(framehost.BrowserJS)}})
	if err != nil {
		t.Fatal(err)
	}
	content, err := distribution.Open(distribution.Config{Directory: filepath.Join(directory, "releases"), Admission: db.Admission, Trust: func(_ context.Context, source distribution.Source, _ distribution.Manifest, _ distribution.Lock) error {
		if !filepath.IsAbs(source.Location) {
			return distribution.ErrUntrusted
		}
		return nil
	}, Retained: func(ctx context.Context, digest framevalue.Digest) (bool, error) {
		states, err := db.Installations.List(ctx)
		if err != nil {
			return false, err
		}
		for _, state := range states {
			if state.Selected.Digest().String() == digest.String() || state.Staged != nil && state.Staged.Digest().String() == digest.String() {
				return true, nil
			}
		}
		return false, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	releases, err := New(content, filepath.Join(directory, "data"), db.PluginReleases)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := host.Close(context.Background())
		if err != nil {
			t.Error(err)
		}
		if err := content.Close(context.Background()); err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		_ = filepath.WalkDir(directory, func(name string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(name, 0700)
			}
			return err
		})
	})
	return releases, &installationFixture{InstallationStore: db.Installations, host: host, catalog: db.PluginReleases}, db.MCPServers
}

type installationFixture struct {
	*persistence.InstallationStore
	host    framehost.Host
	catalog releaseCatalog
}

// The profile is registered to validate declarations. Metadata tests must never
// execute code; real process and browser drivers have their own integration tests.
type unopenedProfile framehost.Profile

func (p unopenedProfile) Profile() framehost.Profile { return framehost.Profile(p) }

func (p unopenedProfile) Prepare(context.Context, framehost.Setup) ([]framehost.ExportValue, error) {
	return nil, errors.New("metadata fixture attempted execution")
}
