package pluginpackage

import (
	"bytes"
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

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	sdkplugin "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

const portableManifest = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.content","version":"1.0.0"}`

// The profile is registered to validate declarations. Metadata tests must never
// execute code; real process and browser drivers have their own integration tests.
type unopenedProfile qimu.Profile

func (p unopenedProfile) Profile() qimu.Profile { return qimu.Profile(p) }
func (p unopenedProfile) Prepare(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
	return nil, errors.New("metadata fixture attempted execution")
}

type installationFixture struct {
	*persistence.InstallationStore
	host    *qimu.Host
	catalog releaseCatalog
}

func (s *installationFixture) Save(ctx context.Context, installation *plugin.Installation) error {
	release, err := s.catalog.Get(ctx, installation.Selected())
	if err != nil {
		return err
	}
	projected, err := plugins.ProjectFrameworkRelease(release)
	if err != nil {
		return err
	}
	if err := plugins.RegisterFrameworkRelease(s.host, projected); err != nil {
		return err
	}
	encoded, err := plugins.EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return err
	}
	_, found, err := s.Get(ctx, installation.ID())
	if err != nil {
		return err
	}
	config := qimu.InstallationConfig{ID: qimu.InstallationID(installation.ID().String()), Plugin: projected.Plugin, Configuration: encoded, Enabled: installation.Enabled()}
	if found {
		original, ok := s.host.Installation(config.ID)
		if !ok {
			return qimu.ErrUnavailable
		}
		return s.host.Update(ctx, original, config)
	}
	_, err = s.host.Install(ctx, config)
	return err
}

func testReleases(t *testing.T) *Releases { releases, _, _ := testReleaseStore(t); return releases }
func testReleaseStore(t *testing.T) (*Releases, *installationFixture, *persistence.MCPServerStore) {
	t.Helper()
	directory := t.TempDir()
	db, err := persistencefixture.Open(t, t.Context(), filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := qimu.NewHost(t.Context(), qimu.HostConfig{Admission: db.Admission, Epoch: db.Qimu.Epoch(), Installations: db.Qimu, ExecutionProtection: db.Qimu, Drivers: []qimu.Driver{unopenedProfile(qimu.MCPProcess), unopenedProfile(qimu.BrowserJS)}})
	if err != nil {
		t.Fatal(err)
	}
	content, err := distribution.Open(distribution.Config{Directory: filepath.Join(directory, "releases"), Admission: db.Admission, Trust: func(_ context.Context, source distribution.Source, _ distribution.Manifest, _ distribution.Lock) error {
		if !filepath.IsAbs(source.Location) {
			return distribution.ErrUntrusted
		}
		return nil
	}, Retained: func(ctx context.Context, digest qimu.Digest) (bool, error) {
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
func flameDeclaration(fields string) string {
	if fields != "" {
		fields = "," + fields
	}
	return `{"protocol":"` + sdkplugin.DeclarationProtocol + `"` + fields + `}`
}
func relocateTestRelease(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if err := os.Chmod(source, mode|0200); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		return errors.Join(err, os.Chmod(source, mode))
	}
	return os.Chmod(destination, mode)
}

type afterReadStore struct {
	plugins.Store
	after func()
}

func (s *afterReadStore) Get(ctx context.Context, id identity.InstallationID) (plugin.Snapshot, bool, error) {
	value, found, err := s.Store.Get(ctx, id)
	if err == nil && s.after != nil {
		call := s.after
		s.after = nil
		call()
	}
	return value, found, err
}

func TestNeutralReleaseOwnsIdentityAndCandidateContentClaim(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "resource.txt": "original"})
	var archive bytes.Buffer
	built, err := BuildRelease(t.Context(), source, BuildTarget{}, &archive)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := releases.Materialize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = candidate.Discard() })
	if candidate.Release().Digest().String() != built.Digest.String() {
		t.Fatal("Flame invented a second release identity")
	}
	if err := releases.Reclaim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := candidate.Publish(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := releases.Reclaim(t.Context()); err != nil {
		t.Fatal(err)
	}
	lease, err := releases.content.Acquire(t.Context(), built.Digest)
	if err != nil {
		t.Fatal("candidate lost content before admission:", err)
	}
	_ = lease.Close()
	if err := candidate.Discard(); err != nil {
		t.Fatal(err)
	}
	if err := releases.Reclaim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := releases.content.Acquire(t.Context(), built.Digest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unretained content survived: %v", err)
	}
	if _, err := releases.catalog.Get(t.Context(), candidate.Release().Digest()); err != nil {
		t.Fatalf("immutable original metadata was lost: %v", err)
	}
	file := filepath.Join(t.TempDir(), "release.zip")
	if err := os.WriteFile(file, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	fromArchive, err := releases.Materialize(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer fromArchive.Discard()
	if fromArchive.Release().Digest() != candidate.Release().Digest() {
		t.Fatal("directory and neutral ZIP have different identities")
	}
}

func TestLegacyContentAndInstalledAmbientTransportsAreRefused(t *testing.T) {
	releases := testReleases(t)
	if _, err := releases.Materialize(t.Context(), writePackage(t, map[string]string{"plugin.json": portableManifest})); err == nil {
		t.Fatal("legacy tree without neutral manifest was admitted")
	}
	for _, server := range []string{`{"type":"stdio","command":"sh"}`, `{"type":"streamable-http","url":"https://example.test/mcp"}`} {
		source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":` + server + `}}`})
		if _, err := publishPackage(t.Context(), releases, source); !errors.Is(err, plugin.ErrInvalid) {
			t.Fatalf("unsupported installed profile admitted: %s: %v", server, err)
		}
	}
}

func TestNativeConnectionRetainsOriginalContentAndDigest(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"./bin/backend"}}}`, "bin/backend": "native fixture"})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	launch, found, err := releases.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
	if err != nil || !found {
		t.Fatalf("connection: %v, %v", found, err)
	}
	defer launch.Retire.Close()
	if launch.Stdio.ExpectedExecutable.String() != qimu.Hash([]byte("native fixture")).String() {
		t.Fatal("launch re-hashed mutable executable as new authority")
	}
	if err := releases.Reclaim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launch.Stdio.Command); err != nil {
		t.Fatal("live original connection lost bytes:", err)
	}
	if err := launch.Retire.Close(); err != nil {
		t.Fatal(err)
	}
	if err := releases.Reclaim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launch.Stdio.Command); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released connection retained content: %v", err)
	}
}

func TestNeutralProjectionCannotAddAnExecutableUnit(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest})
	built, err := BuildRelease(t.Context(), source, BuildTarget{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	manifest := built.Manifest
	manifest.Units = append(manifest.Units, distribution.Unit{ID: "invented", Profile: qimu.NativeGo})
	var output bytes.Buffer
	lock, err := distribution.Build(t.Context(), source, manifest, &output)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Release == built.Digest {
		t.Fatal("declaration mutation retained identity")
	}
	archive := filepath.Join(t.TempDir(), "invented.zip")
	if err := os.WriteFile(archive, output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := releases.Materialize(t.Context(), archive); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("neutral graph escaped Flame consumer contract: %v", err)
	}
}

func TestBackendDirectoryValidationPreservesFilesystemFailure(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	server := plugin.Server{Name: testsupport.ServerName("backend"), Transport: mcpserver.TransportStdio, Command: "fixture", Dir: "./missing"}
	err = inspectServerFiles(root, server)
	if !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing backend directory lost its category or filesystem cause: %v", err)
	}
}

func TestManifestFailurePreservesTheJSONCause(t *testing.T) {
	releases := testReleases(t)
	_, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": `{"name":"first","name":"second"}`}))
	var syntax *jsontext.SyntacticError
	if !errors.Is(err, plugin.ErrInvalid) || !errors.As(err, &syntax) {
		t.Fatalf("manifest failure discarded its validation category or JSON cause: %v", err)
	}
}

func TestManifestKeywordsCannotDefaultNullToText(t *testing.T) {
	releases := testReleases(t)
	manifest := strings.TrimSuffix(portableManifest, "}") + `,"keywords":[null]}`
	if _, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": manifest})); err == nil {
		t.Fatal("manifest admitted a null keyword as an empty string")
	}
}

func TestRetiredViewsDeclarationIsRefused(t *testing.T) {
	r := testReleases(t)
	manifest := `{"$schema":"` + manifestSchema + `","name":"test.retired","extensions":{"` + plugin.Namespace + `":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`
	source := writePackage(t, map[string]string{
		"plugin.json": manifest,
		"io.github.tangerg.flame/declaration.json": flameDeclaration(`"contributes":{"views":[{"id":"schedules","type":"schedules","entry":"view.html"}]}`),
		"view.html": "<!doctype html><p>Retired</p>",
	})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 || declaration.Diagnostics[0].Code != plugin.DiagnosticInvalidDeclaration {
		t.Fatalf("retired vocabulary was accepted: %+v", declaration)
	}
}

func TestInvalidRendererDoesNotWithdrawIndependentContributions(t *testing.T) {
	for _, renderer := range []string{
		`{"id":"broken","title":"Broken","placement":"workspace","scope":"workspace","renderer":{"kind":"webview","entry":"./io.github.tangerg.flame/ui/index.html","bridgeVersion":1},"actions":["missing"]}`,
		`{"id":"broken","unknown":true}`,
	} {
		t.Run(renderer, func(t *testing.T) {
			r := testReleases(t)
			manifest := `{"$schema":"` + manifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`
			declaration := flameDeclaration(`"contributes":{"themes":[{"id":"healthy","title":"Healthy","scheme":"dark","colors":{"background":"#101010"}}]},"components":[` + renderer + `]`)
			source := writePackage(t, map[string]string{"plugin.json": manifest, "io.github.tangerg.flame/declaration.json": declaration, "io.github.tangerg.flame/ui/index.html": "<html></html>"})
			release, err := publishPackage(t.Context(), r, source)
			if err != nil || len(release.Declaration().Themes) != 1 || len(release.Declaration().Diagnostics) != 1 {
				t.Fatalf("component admission = %+v, %v", release, err)
			}
		})
	}
}
