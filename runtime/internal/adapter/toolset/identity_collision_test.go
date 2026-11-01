package toolset

import (
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestResolverQualifiesCollidingRemoteIdentities(t *testing.T) {
	for _, test := range []struct {
		name      string
		tools     []toolcontract.Tool
		collision string
	}{
		{"reserved built-in", []toolcontract.Tool{mcpToolStub{name: "read_skill_resource", server: "read", remote: "skill_resource"}}, "read_skill_resource"},
		{"available built-in", []toolcontract.Tool{mcpToolStub{name: "read_shell_output", server: "read", remote: "shell_output"}}, "read_shell_output"},
		{"remote pair", []toolcontract.Tool{mcpToolStub{name: "a_b_c", server: "a_b", remote: "c"}, mcpToolStub{name: "a_b_c", server: "a", remote: "b_c"}}, "a_b_c"},
	} {
		t.Run(test.name, func(t *testing.T) {
			built, err := Build(BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			closeBuiltToolset(t, built)
			built.Resolver.SetMCPTools(append(test.tools, mcpToolStub{name: "other_read", server: "other", remote: "read"}), nil)
			manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
			if err != nil {
				t.Fatalf("collision prevented manifest construction: %v", err)
			}
			t.Cleanup(func() {
				if err := manifest.Close(); err != nil {
					t.Error(err)
				}
			})
			for _, executable := range manifestTools(manifest) {
				ref, err := Identify(executable)
				if err != nil {
					t.Fatal(err)
				}
				if ref.Kind() != domaintool.BuiltInKind && executable.Definition().Name == test.collision {
					t.Errorf("colliding remote tool remained executable: %s", test.collision)
				}
			}
			if test.name == "available built-in" && !definitionNames(manifestTools(manifest))[test.collision] {
				t.Fatal("collision removed the built-in")
			}
			advertised, conflicts, err := built.Resolver.MCPTools(nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(advertised) != len(test.tools)+1 {
				t.Fatalf("catalog advertised %d tools, want every live tool", len(advertised))
			}
			for _, executable := range test.tools {
				ref, err := Identify(executable)
				if err != nil {
					t.Fatal(err)
				}
				if !definitionNames(manifestTools(manifest))[ref.PublishedName(true)] {
					t.Errorf("missing source-qualified executable for %s", ref)
				}
				if len(conflicts[ref]) == 0 {
					t.Errorf("diagnostic does not identify %s: %v", ref, conflicts)
				}
			}
			if !definitionNames(manifestTools(manifest))["other_read"] {
				t.Fatal("collision removed unrelated tool")
			}
			if _, err := toolcontract.NewRegistry(manifestTools(manifest)...); err != nil {
				t.Fatalf("manifest is not executable: %v", err)
			}
		})
	}
}

func TestRetainedInstallationToolsKeepDistinctNamesAcrossCatalogChanges(t *testing.T) {
	raw := mcpToolStub{name: "github_read", server: "github", remote: "read"}
	makeTool := func() releasedMCPToolStub {
		id := testsupport.InstallationID(t)
		source, err := mcpserver.InstallationSource(id, testsupport.Digest("1"), testsupport.Digest("2"), testsupport.Digest("3"))
		if err != nil {
			t.Fatal(err)
		}
		return releasedMCPToolStub{mcpToolStub: raw, source: source}
	}
	first, second := makeTool(), makeTool()
	firstRef, err := Identify(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := Identify(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]toolcontract.Tool{{first, second}, {second, first}, {first}} {
		builder := manifestBuilder{deferred: values}
		if err := builder.bindModelNames(&Retained{MCP: []domaintool.Ref{firstRef, secondRef}}); err != nil {
			t.Fatal(err)
		}
		discovery, err := NewDiscovery(builder.deferred)
		if err != nil {
			t.Fatal(err)
		}
		names := discovery.DeferredToolNames()
		for _, executable := range builder.deferred {
			ref, err := Identify(executable)
			if err != nil {
				t.Fatal(err)
			}
			if executable.Definition().Name != ref.PublishedName(true) || !slices.Contains(names, ref.PublishedName(true)) {
				t.Fatalf("unaddressable retained tool %s: %v", ref, names)
			}
		}
	}
}
