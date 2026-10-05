package delivery

import (
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestToolReferenceRejectsInvalidMCPIdentity(t *testing.T) {
	files, upper := wireUserServer("files"), wireUserServer("Files")
	installed := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation}, Name: "files"}
	userWithInstallation := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser, InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30"}, Name: "files"}
	for _, ref := range []protocol.ToolRef{
		{Type: protocol.ToolRefMCP, Server: &files, Name: "tool/name"},
		{Type: protocol.ToolRefMCP, Server: &upper, Name: "read"},
		{Type: protocol.ToolRefMCP, Server: &installed, Name: "read"},
		{Type: protocol.ToolRefMCP, Server: &userWithInstallation, Name: "read"},
		{Type: protocol.ToolRefMCP, Name: "read"},
	} {
		if _, err := toolRefFromWire(ref); err == nil {
			t.Fatalf("invalid reference = %v", err)
		}
	}
}

// Every variant crosses the boundary as itself: a reference decoded from its
// own projection is the same reference, whatever its origin.
func TestToolReferenceRoundTripsEveryVariant(t *testing.T) {
	installation, err := resourceid.ParseInstallation("eeb329cd-c7ce-40c9-bd90-6821fef06d30")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := mcpserver.InstallationOrigin(installation)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := mcpserver.NewID(origin, testsupport.ServerName("files"))
	if err != nil {
		t.Fatal(err)
	}
	builtIn, err := tool.BuiltIn(tool.Shell)
	if err != nil {
		t.Fatal(err)
	}
	a2a, err := tool.A2A("planner")
	if err != nil {
		t.Fatal(err)
	}
	refs := []tool.Ref{builtIn, a2a}
	for _, server := range []mcpserver.ID{testsupport.UserMCPServer("files"), installed} {
		ref, err := tool.MCP(server, testsupport.RemoteToolName("read"))
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	covered := map[tool.RefKind]bool{}
	for _, ref := range refs {
		wire, err := presentToolRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.ValidateWireTree(wire); err != nil {
			t.Fatalf("projection of %s is not a valid wire value: %v", ref, err)
		}
		decoded, err := toolRefFromWire(wire)
		if err != nil || decoded != ref {
			t.Fatalf("round trip %s = %v, %v", ref, decoded, err)
		}
		covered[ref.Kind()] = true
	}
	for _, kind := range tool.RefKinds() {
		if !covered[kind] {
			t.Errorf("round trip does not cover %s", kind)
		}
	}
}
