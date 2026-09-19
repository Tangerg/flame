package toolset

import (
	"slices"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// resolveRootManifest builds one exact root visibility snapshot.
func resolveRootManifest(t *testing.T, mcpTools []toolcontract.Tool) Manifest {
	t.Helper()
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools(mcpTools)

	manifest, err := built.Resolver.Manifest(t.Context(), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	return manifest
}

func TestResolverOffersSearchToolsOverDeferredCatalog(t *testing.T) {
	mcpTools := []toolcontract.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	}
	resolved := manifestTools(resolveRootManifest(t, mcpTools))

	var search toolcontract.Tool
	names := make(map[string]bool, len(resolved))
	for _, tool := range resolved {
		names[tool.Definition().Name] = true
		if tool.Definition().Name == domaintool.SearchTools {
			search = tool
		}
	}

	// The MCP tools stay resolvable (in the set) AND a search_tools tool is added.
	if !names["files_read"] || !names["files_write"] {
		t.Fatalf("MCP tools must remain resolvable: %v", names)
	}
	if search == nil {
		t.Fatal("search_tools is unavailable")
	}
	var promoted []string
	ctx := WithToolAdvertiser(t.Context(), func(names ...string) error {
		promoted = append(promoted, names...)
		return nil
	})
	output, err := callTextTool(ctx, search, `{"query":"select:files_read,files_write,lsp"}`)
	if err != nil {
		t.Fatalf("search_tools: %v", err)
	}
	want := []string{"files_read", "files_write", "lsp"}
	if !slices.Equal(promoted, want) {
		t.Fatalf("promoted tools = %v, want %v", promoted, want)
	}
	for _, name := range want {
		if !strings.Contains(output, name) {
			t.Errorf("discovery output omits promoted tool %q: %s", name, output)
		}
	}
}

func TestResolverDefersRuntimeToolsWithoutMCP(t *testing.T) {
	manifest := resolveRootManifest(t, nil)
	advertised := definitionNames(manifest.Visible)
	for _, direct := range []string{domaintool.Read, domaintool.Glob, domaintool.Grep, domaintool.ApplyPatch, domaintool.Shell, domaintool.SearchTools} {
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
