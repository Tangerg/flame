package pluginpackage

import (
	"archive/zip"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func testReleases(t *testing.T) *Releases {
	releases, _, _ := testReleaseStore(t)
	return releases
}

func testReleaseStore(t *testing.T) (*Releases, *sqlite.InstallationStore, *sqlite.MCPServerStore) {
	t.Helper()
	directory := t.TempDir()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := New(directory, sqlite.NewReleaseStore(db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return r, sqlite.NewInstallationStore(db), sqlite.NewMCPServerStore(db)
}
func writePackage(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const portableManifest = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.content"}`

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

func TestMalformedArchiveRetainsFormatCauseAndMissingSourceIsNotInvalidInput(t *testing.T) {
	releases := testReleases(t)
	source := filepath.Join(t.TempDir(), "package.zip")
	if err := os.WriteFile(source, []byte("invalid archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishPackage(t.Context(), releases, source); !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("malformed archive lost its validation category or format cause: %v", err)
	}
	if _, err := publishPackage(t.Context(), releases, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) || errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("missing source lost its filesystem cause or became invalid input: %v", err)
	}
}

func TestUnsupportedArchiveCompressionPreservesAlgorithmCause(t *testing.T) {
	releases := testReleases(t)
	source := filepath.Join(t.TempDir(), "package.zip")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	_, createErr := archive.CreateRaw(&zip.FileHeader{Name: "plugin.json", Method: 99})
	if err := errors.Join(createErr, archive.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err := publishPackage(t.Context(), releases, source); !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, zip.ErrAlgorithm) {
		t.Fatalf("unsupported archive compression lost its category or algorithm cause: %v", err)
	}
}

func TestManifestKeywordsCannotDefaultNullToText(t *testing.T) {
	releases := testReleases(t)
	manifest := strings.TrimSuffix(portableManifest, "}") + `,"keywords":[null]}`
	if _, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": manifest})); err == nil {
		t.Fatal("manifest admitted a null keyword as an empty string")
	}
}

func TestInvalidContributionMapPreservesIndependentInputs(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `"invalid"`} {
		t.Run(raw, func(t *testing.T) {
			releases := testReleases(t)
			release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1,"inputs":[{"id":"option","server":"backend","target":"env","key":"OPTION"}],"contributes":` + raw + `}}}`,
				"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"fixture"}}}`,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if len(release.Declaration().Inputs) != 1 || len(release.Declaration().Diagnostics) != 1 || release.Declaration().Diagnostics[0] != (plugin.Diagnostic{Component: plugin.Component{Kind: plugin.ComponentExtensionField, Name: "contributes"}, Code: plugin.DiagnosticInvalidDeclaration}) {
				t.Fatalf("invalid contribution map escaped its component boundary: %+v", release)
			}
		})
	}
}

// Capability requests had no enforcement point and are no longer part of the
// Flame namespace. A package that still declares them is told so, and the
// independent members of the namespace are still admitted.
func TestUnsupportedFlameFieldIsReportedNotAdmitted(t *testing.T) {
	releases := testReleases(t)
	release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{
		"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1,"requests":[{"capability":"tools.invoke","targets":["backend/read"]}],"inputs":[{"id":"option","server":"backend","target":"env","key":"OPTION"}]}}}`,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"fixture"}}}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	want := []plugin.Diagnostic{{Component: plugin.Component{Kind: plugin.ComponentExtensionField, Name: "requests"}, Code: plugin.DiagnosticUnknownField}}
	if !reflect.DeepEqual(declaration.Diagnostics, want) || len(declaration.Inputs) != 1 || len(declaration.Servers) != 1 {
		t.Fatalf("unsupported Flame field = %+v", declaration)
	}
}

func TestNullCannotBecomeAnImplicitDeclarationDefault(t *testing.T) {
	for _, test := range []struct {
		name, extension, mcp string
		servers              int
	}{
		{"input flag", `"inputs":[{"id":"option","server":"backend","target":"env","key":"OPTION","required":null}]`, `{"type":"stdio","command":"fixture"}`, 2},
		{"theme colors", `"contributes":{"themes":[{"id":"theme","title":"Theme","scheme":"dark","colors":null}]}`, `{"type":"stdio","command":"fixture"}`, 2},
		{"environment value", `"inputs":[]`, `{"type":"stdio","command":"fixture","env":{"OPTION":null}}`, 1},
		{"argument", `"inputs":[]`, `{"type":"stdio","command":"fixture","args":[null]}`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			releases := testReleases(t)
			release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1,` + test.extension + `}}}`,
				"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":` + test.mcp + `,"healthy":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if len(release.Declaration().Inputs) != 0 || len(release.Declaration().Themes) != 0 || len(release.Declaration().Diagnostics) == 0 {
				t.Fatalf("null was admitted as a declaration default: %+v", release)
			}
			if len(release.Declaration().Servers) != test.servers {
				t.Fatalf("invalid backend was retained or healthy sibling withdrawn: %+v", release)
			}
		})
	}
}

func TestOptionalEmptyInputsReachTheirDeclaredBindings(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{
		"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1,"inputs":[{"id":"environment","server":"process","target":"env","key":"OPTION"},{"id":"header","server":"remote","target":"header","key":"X-Option","secret":true}]}}}`,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"process":{"type":"stdio","command":"fixture"},"remote":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
	})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		change plugin.ValueChange
		set    bool
	}{{plugin.SetValue(""), true}, {plugin.ClearValue(), false}} {
		if err := installation.Configure(release, plugin.Configuration{Values: map[string]plugin.ValueChange{"environment": test.change, "header": test.change}}); err != nil {
			t.Fatal(err)
		}
		servers, err := realizedServers(t.Context(), releases, installation, release)
		if err != nil {
			t.Fatal(err)
		}
		for _, server := range servers {
			bindings, key := server.Env, "OPTION"
			if server.Name.String() == "remote" {
				bindings, key = server.Headers, "X-Option"
			}
			bound, exists := bindings[key]
			if exists != test.set || bound != "" {
				t.Fatalf("optional input presence lost for %s: %t, %q", server.Name, exists, bound)
			}
		}
	}
}

func TestCachedIntegrityRefusesAReleaseRootAlias(t *testing.T) {
	releases := testReleases(t)
	release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": portableManifest}))
	if err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	retained := root + "-retained"
	if err := os.Rename(root, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(retained, root); err != nil {
		t.Fatal(err)
	}
	capability, _, err := releases.verifiedRoot(t.Context(), release.Digest())
	if capability != nil {
		_ = capability.Close()
	}
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("cached integrity admitted a redirected root: %v", err)
	}
}

func TestDeclaredSearchPathDoesNotCompeteWithHostInheritance(t *testing.T) {
	releases := testReleases(t)
	release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"fixture","env":{"Path":"declared"}}}}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	servers, err := realizedServers(t.Context(), releases, installation, release)
	if err != nil || len(servers) != 1 {
		t.Fatalf("declared descriptor = %d, %v", len(servers), err)
	}
	if _, inherited := servers[0].Env["PATH"]; inherited || servers[0].Env["Path"] != "declared" {
		t.Fatal("host inheritance introduced a competing portable search path")
	}
}

func TestMCPRegistryRetainsUnavailableDeclaredSources(t *testing.T) {
	releases, store, users := testReleaseStore(t)
	user, err := mcpserver.ParseServerName("user")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.Save(t.Context(), mcpserver.Server{Source: mcpserver.UserSource(), Name: user, Transport: mcpserver.TransportStdio, Command: "fixture"}); err != nil {
		t.Fatal(err)
	}
	source := writePackage(t, map[string]string{"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1}}}`, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"streamable-http","url":"https://example.test/mcp"}}}`})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(users, store, releases.catalog, releases)
	if err != nil {
		t.Fatal(err)
	}
	name, err := installation.ServerID(testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := tool.MCP(name, mustRemoteToolName(t, "read"))
	if err != nil {
		t.Fatal(err)
	}
	authorities := toolset.NewAuthorities(registry.Definition, nil)
	before, found, err := authorities.Fingerprint(t.Context(), ref)
	if err != nil || !found {
		t.Fatalf("initial authority = %t, %v", found, err)
	}
	root, err := releases.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	sources, err := registry.Catalog(t.Context())
	if err != nil || len(sources) != 2 || sources[0].Availability != mcpapp.SourceAvailable || sources[1].Availability != mcpapp.SourceUnavailableRelease {
		t.Fatalf("unavailable declared source = %+v, %v", sources, err)
	}
	if _, found, err := registry.Dispatchable(t.Context(), sources[1].Server.ID()); found || !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("unavailable source admitted dispatch: %t, %v", found, err)
	}
	current, found, err := authorities.Fingerprint(t.Context(), ref)
	if err != nil || !found || current != before {
		t.Fatalf("unavailable bytes obscured standing authority: %t, %v", found, err)
	}
	installation.Revoke()
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	current, found, err = authorities.Fingerprint(t.Context(), ref)
	if err != nil || !found || current != before {
		t.Fatalf("revoking the same code advanced its standing authority: %t, %v", found, err)
	}
}

func mustRemoteToolName(t *testing.T, text string) mcpserver.RemoteToolName {
	t.Helper()
	name, err := mcpserver.ParseRemoteToolName(text)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestMCPCatalogRetainsAnUnavailableBackendAndItsHealthyPeer(t *testing.T) {
	releases, store, users := testReleaseStore(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"broken":{"type":"stdio","command":"fixture","cwd":"${PLUGIN_DATA}/work"},"healthy":{"type":"streamable-http","url":"https://example.test/mcp"}}}`})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(users, store, releases.catalog, releases)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.Catalog(t.Context())
	if err != nil || len(catalog) != 2 {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	for _, source := range catalog {
		want := mcpapp.SourceAvailable
		if source.Server.Name.String() == "broken" {
			want = mcpapp.SourceUnavailableBackend
		}
		if source.Availability != want {
			t.Fatalf("source %s availability = %s", source.Server.Name, source.Availability)
		}
	}
	if _, err := os.Stat(releases.dataRoot(installation.ID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("catalog read prepared the backend: %v", err)
	}
	if err := releases.Prepare(t.Context(), installation, release); err != nil {
		t.Fatalf("prepare = %v", err)
	}
	catalog, err = registry.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range catalog {
		if source.Availability != mcpapp.SourceAvailable {
			t.Fatalf("prepared source is unavailable: %+v", source)
		}
	}
}

func TestMaterializationFreezesBytesAndDetectsTampering(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "original"})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "notes.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.readResource(t.Context(), installation, "notes.txt", MaxFileBytes)
	if err != nil || string(got) != "original" {
		t.Fatalf("immutable resource = %q, %v", got, err)
	}
	root, _ := r.Root(release.Digest())
	file := filepath.Join(root, "notes.txt")
	if err = os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = r.readResource(t.Context(), installation, "notes.txt", MaxFileBytes); err == nil {
		t.Fatal("served a changed content-addressed resource")
	}
}

func TestColdResourceReadAfterAdmissionPressure(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "original"})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 2*plugin.MaxInstallations + 1 {
		if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte(fmt.Sprintf("replacement %d", index)), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := publishPackage(t.Context(), r, source); err != nil {
			t.Fatal(err)
		}
	}
	content, err := r.readResource(t.Context(), installation, "notes.txt", MaxFileBytes)
	if err != nil || string(content) != "original" {
		t.Fatalf("cold immutable resource = %q, %v", content, err)
	}
}
func TestPortableFailuresRemainIndependent(t *testing.T) {
	for _, extension := range []string{`"invalid"`, `{"io.github.tangerg.flame":{"apiVersion":9}}`, `{"io.github.tangerg.flame":{"apiVersion":1,"unknown":true}}`, `{"other.vendor":{"anything":true}}`} {
		t.Run(extension, func(t *testing.T) {
			r := testReleases(t)
			manifest := strings.TrimSuffix(portableManifest, "}") + `,"extensions":` + extension + `}`
			source := writePackage(t, map[string]string{"plugin.json": manifest, "mcp.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"good":{"type":"stdio","command":"test-server"},"broken":{"type":"unknown"}}}`, "skills/review/SKILL.md": "---\nname: review\ndescription: Review a result\n---\nRead the results carefully.\n"})
			release, err := publishPackage(t.Context(), r, source)
			if err != nil {
				t.Fatal(err)
			}
			if len(release.Declaration().Servers) != 1 || len(release.Declaration().Skills) != 1 || len(release.Declaration().Diagnostics) == 0 {
				t.Fatalf("component admission = %+v", release)
			}
		})
	}
}

func TestVerifiedResourceRetainsItsDirectoryAuthority(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "original"})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	verified, _, err := r.verifiedRoot(t.Context(), installation.Selected())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = verified.Close() })
	directory, err := r.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory, directory+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	content, err := read(t.Context(), verified, "notes.txt", MaxFileBytes)
	if err != nil || string(content) != "original" {
		t.Fatalf("verified resource = %q, %v; want original directory", content, err)
	}
	if _, err := r.readResource(t.Context(), installation, "notes.txt", MaxFileBytes); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("replacement release admission = %v, want unavailable", err)
	}
}
func TestArchivesRejectEscapesAndAmbiguousDestinations(t *testing.T) {
	for _, names := range [][]string{{"plugin.json", "../escaped"}, {"plugin.json", "A/file", "a/other"}, {"plugin.json", "same", "same"}, {"plugin.json", "link"}, {"plugin.json", "a/", "a/"}, {"plugin.json", "NUL.txt"}, {"plugin.json", "COM¹.txt"}, {"plugin.json", "nested/LPT².txt"}, {"plugin.json", "directory./file"}, {"plugin.json", "Café/file", "Cafe\u0301/other"}} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			r := testReleases(t)
			archive := filepath.Join(t.TempDir(), "package.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			for _, name := range names {
				header := &zip.FileHeader{Name: name}
				if name == "link" {
					header.SetMode(os.ModeSymlink | 0700)
				}
				writer, err := w.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				body := "value"
				if name == "plugin.json" {
					body = portableManifest
				}
				if !strings.HasSuffix(name, "/") {
					if _, err = writer.Write([]byte(body)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = errors.Join(w.Close(), f.Close()); err != nil {
				t.Fatal(err)
			}
			if _, err = publishPackage(t.Context(), r, archive); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("unsafe archive lost its validation category: %v", err)
			}
		})
	}
}
func TestInstallationServersDoNotLaunchBeforeAdmission(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"good":{"type":"stdio","command":"test-server","args":["${PLUGIN_ROOT}","${PLUGIN_DATA}"],"cwd":"${PLUGIN_ROOT}"}}}`})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	i, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err = i.Enable(release); !errors.Is(err, plugin.ErrUnapproved) {
		t.Fatalf("enable without approval = %v", err)
	}
	before, err := observeBackends(t.Context(), r, i, release)
	if err != nil || len(before.Servers) != 1 || before.Servers[0].Enabled {
		t.Fatalf("unadmitted projection = %+v, %v", before, err)
	}
	if err = i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err = i.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := r.Prepare(t.Context(), i, release); err != nil {
		t.Fatal(err)
	}
	after, err := observeBackends(t.Context(), r, i, release)
	if err != nil || !after.Servers[0].Enabled || after.Servers[0].Source.Origin() != mustInstallationOrigin(t, i.ID()) || after.Servers[0].Args[0] != after.Servers[0].Dir {
		t.Fatalf("admitted projection = %+v, %v", after, err)
	}
}

func TestMCPUnionPresenceCannotCrossTransportOrHideNull(t *testing.T) {
	for _, component := range []string{`{"type":"stdio","command":"server","url":""}`, `{"type":"stdio","command":"server","env":null}`, `{"type":"streamable-http","url":"https://example.test/mcp","args":[]}`, `{"type":"stdio","command":"server","env":{"BAD=KEY":"value"}}`} {
		t.Run(component, func(t *testing.T) {
			releases := testReleases(t)
			source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"good":{"type":"stdio","command":"server"},"bad":` + component + `}}`})
			release, err := publishPackage(t.Context(), releases, source)
			if err != nil {
				t.Fatal(err)
			}
			if len(release.Declaration().Servers) != 1 || release.Declaration().Servers[0].Name.String() != "good" || len(release.Declaration().Diagnostics) != 1 {
				t.Fatalf("transport admission: %+v", release)
			}
		})
	}
}

func TestShippedTrajectoryPackageIsAdmittedWithoutExecutingCode(t *testing.T) {
	releases := testReleases(t)
	source, err := filepath.Abs("../../../../../examples/plugins/trajectory")
	if err != nil {
		t.Fatal(err)
	}
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(release.Declaration().Themes) != 1 || len(release.Declaration().Skills) != 1 || len(release.Declaration().Diagnostics) != 0 {
		t.Fatalf("example admission: %+v", release)
	}
}

func TestOverlappingAdmissionsShareAnImmutableRelease(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "shared content"})
	ready := make(chan struct{})
	type admission struct {
		release plugin.Release
		err     error
	}
	results := make(chan admission, 2)
	for range 2 {
		go func() {
			<-ready
			release, err := publishPackage(t.Context(), r, source)
			results <- admission{release, err}
		}()
	}
	close(ready)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.release.Digest() != second.release.Digest() {
		t.Fatalf("overlapping admissions: %v, %v", first.err, second.err)
	}
	if _, err := r.verify(t.Context(), first.release.Digest()); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseKeepsItsAdmissionInterpretation(t *testing.T) {
	for _, operation := range []string{"materialize", "cold catalog", "cold backends", "cold connection"} {
		t.Run(operation, func(t *testing.T) {
			r, store, _ := testReleaseStore(t)
			source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"streamable-http","url":"https://example.test/mcp"}}}`})
			root, err := os.OpenRoot(source)
			if err != nil {
				t.Fatal(err)
			}
			digest, _, err := treeDigest(t.Context(), root)
			if err := errors.Join(err, root.Close()); err != nil {
				t.Fatal(err)
			}
			admitted, err := plugin.NewRelease(digest, plugin.Declaration{Name: "test.content", Diagnostics: []plugin.Diagnostic{{Component: plugin.Component{Kind: plugin.ComponentMCPServer, Name: "backend"}, Code: plugin.DiagnosticInvalidDeclaration}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.catalog.Admit(t.Context(), admitted); err != nil {
				t.Fatal(err)
			}
			materialized, err := publishPackage(t.Context(), r, source)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "materialize" {
				if len(materialized.Declaration().Servers) != 0 || len(materialized.Declaration().Diagnostics) != 1 {
					t.Fatalf("reinterpreted an admitted release: %+v", materialized)
				}
				return
			}
			installation, err := plugin.New(testsupport.InstallationID(t), source, admitted)
			if err != nil {
				t.Fatal(err)
			}
			release := admitted
			if err := installation.Approve(release); err != nil {
				t.Fatal(err)
			}
			if err := installation.Enable(release); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), installation); err != nil {
				t.Fatal(err)
			}
			cold, err := New(r.directory, r.catalog)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "cold catalog" {
				servers, err := realizedServers(t.Context(), cold, installation, release)
				if err != nil || len(servers) != 0 {
					t.Fatalf("catalog introduced an unadmitted backend: %+v, %v", servers, err)
				}
				return
			}
			if operation == "cold connection" {
				_, found, err := cold.Server(t.Context(), installation, release, mustServerName(t, "backend"), plugins.Launchable)
				if found || err != nil {
					t.Fatalf("connection introduced an unadmitted backend: %v, %v", found, err)
				}
				return
			}
			backends, err := observeBackends(t.Context(), cold, installation, release)
			if err != nil || len(backends.Servers) != 0 {
				t.Fatalf("cold validation introduced an unadmitted backend: %+v, %v", backends, err)
			}
		})
	}
}

func TestConflictingInputDoesNotWithdrawTheBackend(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{
		"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.content","extensions":{"` + plugin.Namespace + `":{"apiVersion":1,"inputs":[{"id":"tenant","server":"backend","target":"header","key":"x-tenant","secret":true}]}}}`,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"streamable-http","url":"https://example.test/mcp","headers":{"X-Tenant":"fixed"}}}}`,
	})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(release.Declaration().Servers) != 1 || len(release.Declaration().Inputs) != 0 || len(release.Declaration().Diagnostics) != 1 || release.Declaration().Diagnostics[0].Component != (plugin.Component{Kind: plugin.ComponentExtensionField, Name: "inputs"}) {
		t.Fatalf("configuration admission did not isolate the invalid input: %+v", release)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	servers, err := observeBackends(t.Context(), r, installation, release)
	if err != nil || len(servers.Servers) != 1 || servers.Servers[0].Headers["X-Tenant"] != "fixed" {
		t.Fatalf("independent backend was withdrawn: %+v, %v", servers, err)
	}
}

func TestArchiveAdmissionCountsImplicitDirectories(t *testing.T) {
	r := testReleases(t)
	archive := filepath.Join(t.TempDir(), "implicit.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(f)
	manifest, err := writer.Create("plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manifest.Write([]byte(portableManifest)); err != nil {
		t.Fatal(err)
	}
	for index := range MaxFiles / 2 {
		entry, err := writer.Create(fmt.Sprintf("directory-%d/notes.txt", index))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte("bounded")); err != nil {
			t.Fatal(err)
		}
	}
	if err = errors.Join(writer.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err = publishPackage(t.Context(), r, archive); err == nil {
		t.Fatal("admitted a tree beyond its entry limit")
	}
	entries, err := os.ReadDir(r.directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("retained failed staging: %v, %v", entries, err)
	}
}

func TestOversizedFileAdmissionRetiresStaging(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "oversized": ""})
	if err := os.Truncate(filepath.Join(source, "oversized"), MaxFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := publishPackage(t.Context(), r, source); err == nil {
		t.Fatal("admitted an oversized file")
	}
	entries, err := os.ReadDir(r.directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("retained failed staging: %v, %v", entries, err)
	}
}

func TestUnsupportedContributionDoesNotWithdrawIndependentContributions(t *testing.T) {
	for _, view := range []string{
		`{"id":"broken","title":"Broken","placement":"workspace","scope":"workspace","renderer":{"kind":"webview","entry":"./io.github.tangerg.flame/ui/index.html","bridgeVersion":1},"actions":["missing"]}`,
		`{"id":"broken","unknown":true}`,
	} {
		t.Run(view, func(t *testing.T) {
			r := testReleases(t)
			manifest := strings.TrimSuffix(portableManifest, "}") + `,"extensions":{"io.github.tangerg.flame":{"apiVersion":1,"contributes":{"themes":[{"id":"healthy","title":"Healthy","scheme":"dark","colors":{"background":"#101010"}}],"views":[` + view + `]}}}}`
			source := writePackage(t, map[string]string{"plugin.json": manifest, "io.github.tangerg.flame/ui/index.html": "<html></html>"})
			release, err := publishPackage(t.Context(), r, source)
			if err != nil || len(release.Declaration().Themes) != 1 || len(release.Declaration().Diagnostics) != 1 {
				t.Fatalf("component admission = %+v, %v", release, err)
			}
		})
	}
}

func TestBackendRealizationIsPureAndIsolatesDirectoryFailures(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"good":{"type":"stdio","command":"server"},"broken":{"type":"stdio","command":"server","cwd":"${PLUGIN_DATA}/blocked/work"}}}`})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	i, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observeBackends(t.Context(), r, i, release); err != nil {
		t.Fatal(err)
	}
	data := r.dataRoot(i.ID())
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created data root: %v", err)
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "blocked"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Prepare(t.Context(), i, release); err == nil {
		t.Fatal("preparation concealed a directory failure")
	}
	backends, err := observeBackends(t.Context(), r, i, release)
	if err != nil || len(backends.Servers) != 1 || backends.Servers[0].Name.String() != "good" || !slices.Equal(backends.Unavailable, []mcpserver.ServerName{mustServerName(t, "broken")}) {
		t.Fatalf("isolated realization = %+v, %v", backends, err)
	}
}

func TestBackendPreparationDoesNotWriteThroughAReplacedDataNamespace(t *testing.T) {
	for _, operation := range []string{"installation", "connection"} {
		t.Run(operation, func(t *testing.T) {
			r := testReleases(t)
			source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"server"}}}`})
			release, err := publishPackage(t.Context(), r, source)
			if err != nil {
				t.Fatal(err)
			}
			installation, err := plugin.New(testsupport.InstallationID(t), source, release)
			if err != nil {
				t.Fatal(err)
			}
			if err := installation.Approve(release); err != nil {
				t.Fatal(err)
			}
			if err := installation.Enable(release); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			namespace := filepath.Dir(r.dataRoot(installation.ID()))
			if err := os.Symlink(outside, namespace); err != nil {
				t.Fatal(err)
			}
			if operation == "installation" {
				if err := r.Prepare(t.Context(), installation, release); err == nil {
					t.Fatal("preparation accepted a replaced data namespace")
				}
			} else if _, _, err := r.Server(t.Context(), installation, release, mustServerName(t, "backend"), plugins.Launchable); err == nil {
				t.Fatal("connection accepted a replaced data namespace")
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("preparation wrote outside its data owner: %v, %v", entries, err)
			}
		})
	}
}

func mustServerName(t *testing.T, raw string) mcpserver.ServerName {
	t.Helper()
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func mustInstallationOrigin(t *testing.T, id resourceid.InstallationID) mcpserver.Origin {
	t.Helper()
	origin, err := mcpserver.InstallationOrigin(id)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

// The portable mcp.json spelling is translated once, at this boundary, into
// the MCP registry vocabulary the release declares. The registry spelling is
// not a portable spelling and is isolated like any unknown transport.
func TestPortableServerSpellingIsTranslatedAtTheBoundary(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"local":{"type":"stdio","command":"./bin/server","cwd":"${PLUGIN_ROOT}/bin"},"remote":{"type":"streamable-http","url":"https://example.test/mcp"},"registry":{"type":"streamableHttp","url":"https://example.test/mcp"}}}`,
		"bin/server":  "#!/bin/sh\n",
	})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	want := []plugin.Server{
		{Name: testsupport.ServerName("local"), Transport: mcpserver.TransportStdio, Command: "./bin/server", Dir: "${PLUGIN_ROOT}/bin"},
		{Name: testsupport.ServerName("remote"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"},
	}
	if !reflect.DeepEqual(declaration.Servers, want) {
		t.Fatalf("translated servers = %+v, want %+v", declaration.Servers, want)
	}
	isolated := plugin.Diagnostic{Component: plugin.Component{Kind: plugin.ComponentMCPServer, Name: "registry"}, Code: plugin.DiagnosticInvalidDeclaration}
	if !slices.Contains(declaration.Diagnostics, isolated) {
		t.Fatalf("registry spelling was not isolated: %+v", declaration.Diagnostics)
	}
}

// publishPackage admits and publishes a package the way an installation change
// does.
func publishPackage(ctx context.Context, r *Releases, source string) (plugin.Release, error) {
	candidate, err := r.Materialize(ctx, source)
	if err != nil {
		return plugin.Release{}, err
	}
	release, err := candidate.Publish(ctx)
	return release, errors.Join(err, candidate.Discard())
}

func realizedServers(ctx context.Context, r *Releases, installation *plugin.Installation, release plugin.Release) ([]mcpserver.Server, error) {
	realization, err := r.Realize(ctx, installation, release)
	if err != nil {
		return nil, err
	}
	servers := make([]mcpserver.Server, 0, len(realization.Sources))
	for _, source := range realization.Sources {
		servers = append(servers, source.Server)
	}
	return servers, nil
}

type observedBackends struct {
	Servers     []mcpserver.Server
	Unavailable []mcpserver.ServerName
}

func observeBackends(ctx context.Context, r *Releases, installation *plugin.Installation, release plugin.Release) (observedBackends, error) {
	realization, err := r.Realize(ctx, installation, release)
	if err != nil {
		return observedBackends{}, err
	}
	if realization.Release != plugins.ReleaseAvailable {
		return observedBackends{}, plugin.ErrUnavailable
	}
	var observed observedBackends
	for _, source := range realization.Sources {
		if source.Availability == mcpapp.SourceAvailable {
			observed.Servers = append(observed.Servers, source.Server)
		}
	}
	observed.Unavailable = realization.UnavailableBackends()
	return observed, nil
}
