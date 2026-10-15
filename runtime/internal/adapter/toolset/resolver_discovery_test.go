package toolset

import (
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// resolveRootManifest builds one exact root visibility snapshot.
func resolveRootManifest(t *testing.T, mcpTools []toolcontract.Tool) Manifest {
	t.Helper()
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools(mcpTools, nil)

	manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	return manifest
}

type deferredNamer interface{ DeferredToolNames() []string }

func TestResolverOffersSearchToolsOverDeferredCatalog(t *testing.T) {
	mcpTools := []toolcontract.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	}
	resolved := manifestTools(resolveRootManifest(t, mcpTools))

	var search deferredNamer
	names := make(map[string]bool, len(resolved))
	for _, tool := range resolved {
		names[tool.Definition().Name] = true
		// Telemetry and policy decorators keep an optional capability reachable
		// only through the wrapping chain, which is where Scope resolves it.
		if d, found, err := toolcontract.Capability[deferredNamer](tool); err == nil && found {
			search = d
		}
	}

	// The MCP tools stay resolvable (in the set) AND a search_tools tool is added.
	if !names["files_read"] || !names["files_write"] {
		t.Fatalf("MCP tools must remain resolvable: %v", names)
	}
	if !names["search_tools"] {
		t.Fatalf("search_tools must be offered when MCP tools exist: %v", names)
	}
	if search == nil {
		t.Fatal("no tool reports deferred names")
	}
	deferred := nameSliceSet(search.DeferredToolNames())
	for _, want := range []string{"files_read", "files_write", "lsp"} {
		if !deferred[want] {
			t.Errorf("deferred names = %v, missing %q", deferred, want)
		}
	}
}

func TestResolverDefersRuntimeToolsWithoutMCP(t *testing.T) {
	manifest := resolveRootManifest(t, nil)
	advertised := definitionNames(manifest.Visible)
	for _, direct := range []string{string(domaintool.Read), string(domaintool.Glob), string(domaintool.Grep), string(domaintool.ApplyPatch), string(domaintool.Shell), string(domaintool.SearchTools)} {
		if !advertised[direct] {
			t.Errorf("initial manifest = %v, missing direct tool %q", advertised, direct)
		}
	}
	for _, deferred := range []string{"lsp"} {
		if advertised[deferred] {
			t.Errorf("initial manifest = %v, unexpectedly advertised deferred tool %q", advertised, deferred)
		}
	}
}

func nameSliceSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

func TestResolverRetainsCapturedMCPAuthorityBeforeDiscovery(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools([]toolcontract.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	}, nil)
	read := testsupport.MCPTool(testsupport.UserMCPServer("files"), "read")
	for _, test := range []struct {
		name        string
		captured    []domaintool.Ref
		read, write bool
	}{
		{"fresh", nil, true, true},
		{"captured empty", []domaintool.Ref{}, false, false},
		{"captured read", []domaintool.Ref{read}, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, test.captured)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := manifest.Close(); err != nil {
					t.Error(err)
				}
			})
			names := definitionNames(manifest.Deferred)
			if names["files_read"] != test.read || names["files_write"] != test.write {
				t.Fatalf("restored executable authority = %v", names)
			}
			var discovery deferredNamer
			for _, executable := range manifest.Visible {
				candidate, found, err := toolcontract.Capability[deferredNamer](executable)
				if err != nil {
					t.Fatal(err)
				}
				if found {
					discovery = candidate
				}
			}
			if discovery == nil {
				t.Fatal("deferred discovery is missing")
			}
			advertised := nameSliceSet(discovery.DeferredToolNames())
			if advertised["files_read"] != test.read || advertised["files_write"] != test.write {
				t.Fatalf("restored discovery authority = %v", advertised)
			}
		})
	}
	fresh, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fresh.Close(); err != nil {
			t.Error(err)
		}
	}()
	names := definitionNames(fresh.Deferred)
	if !names["files_read"] || !names["files_write"] {
		t.Fatalf("restoration changed the shared current catalog: %v", names)
	}
}
