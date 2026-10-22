package toolset

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
)

type releasedMCPToolStub struct {
	mcpToolStub
	source mcpserver.Source
}

func (r releasedMCPToolStub) SourceConfig() mcp.ServerConfig {
	return mcp.ServerConfig{Server: mcpserver.Server{Source: r.source, Name: testsupport.ServerName(r.server)}}
}

func (r releasedMCPToolStub) InstallationActivation() (plugin.Activation, error) {
	id, _ := r.source.Origin().Installation()
	digest, _, _ := r.source.Release()
	return plugin.Activation{Identity: testsupport.Digest("activation"), Dependencies: []plugin.Dependency{{InstallationID: id, Digest: digest}}}, nil
}

// unrealizedMCPTool reports a remote identity without the connection that
// admitted it, so it has no server identity at all.
type unrealizedMCPTool struct{ name, remote string }

func (u unrealizedMCPTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: u.name, InputSchema: []byte(`{"type":"object"}`)}
}
func (unrealizedMCPTool) Call(context.Context, toolcontract.Invocation) (toolcontract.Result, error) {
	return toolcontract.Result{}, nil
}
func (u unrealizedMCPTool) MCPToolIdentity() (string, string) { return "reviews", u.remote }

// Installation MCP tools and package Skills enter one dependency projection,
// so admission never reconstructs dependencies from tool references.
func TestManifestProjectsEveryInstallationRelease(t *testing.T) {
	installation := testsupport.InstallationID(t)
	skill := plugin.Dependency{InstallationID: installation, Digest: testsupport.Digest("skill release")}
	release := testsupport.Digest("server release")
	source, err := mcpserver.InstallationSource(installation, release, testsupport.Digest("authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := oteltool.NewMiddleware(oteltool.MiddlewareConfig{})
	if err != nil {
		t.Fatal(err)
	}
	builder := manifestBuilder{installations: []plugin.Dependency{skill}, deferred: []toolcontract.Tool{
		releasedMCPToolStub{mcpToolStub{name: "reviews_record", server: "reviews", remote: "record"}, source},
		releasedMCPToolStub{mcpToolStub{name: "reviews_list", server: "reviews", remote: "list"}, source},
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
	}}
	manifest, err := builder.manifest(telemetry)
	if err != nil {
		t.Fatal(err)
	}
	want := plugin.CompactDependencies([]plugin.Dependency{skill, {InstallationID: installation, Digest: release}})
	if !slices.Equal(manifest.Installations, want) {
		t.Fatalf("manifest dependencies = %+v, want %+v", manifest.Installations, want)
	}

	builder.deferred = []toolcontract.Tool{unrealizedMCPTool{name: "reviews_record", remote: "record"}}
	if _, err := builder.manifest(telemetry); err == nil {
		t.Fatal("an MCP tool without a realized connection entered the manifest")
	}
}

type transitiveMCPToolStub struct {
	releasedMCPToolStub
	activation *plugin.Activation
}

func (r transitiveMCPToolStub) InstallationActivation() (plugin.Activation, error) {
	return r.activation.Clone(), nil
}

func TestManifestRetainsTransitiveReleasesAndRefusesAChangedGraph(t *testing.T) {
	installation := testsupport.InstallationID(t)
	root := plugin.Dependency{InstallationID: installation, Digest: testsupport.Digest("root")}
	leaf := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("leaf")}
	source, err := mcpserver.InstallationSource(installation, root.Digest, testsupport.Digest("authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	activation := plugin.Activation{Identity: testsupport.Digest("activation"), Dependencies: plugin.CompactDependencies([]plugin.Dependency{root, leaf})}
	executable := transitiveMCPToolStub{releasedMCPToolStub{mcpToolStub{name: "reviews_record", server: "reviews", remote: "record"}, source}, &activation}
	telemetry, err := oteltool.NewMiddleware(oteltool.MiddlewareConfig{})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := (manifestBuilder{visible: []toolcontract.Tool{executable}}).manifest(telemetry)
	if err != nil || !slices.Equal(manifest.Installations, activation.Dependencies) {
		t.Fatalf("manifest lost transitive release: %v, %v", manifest.Installations, err)
	}
	activation.Identity = testsupport.Digest("same release with another binding")
	if _, err := manifest.Visible[0].Call(t.Context(), toolcontract.Invocation{}); !errors.Is(err, plugin.ErrStale) {
		t.Fatalf("changed activation graph dispatched: %v", err)
	}
	if !slices.Contains(manifest.Installations, leaf) {
		t.Fatal("source change rewrote frozen dependencies")
	}
}
