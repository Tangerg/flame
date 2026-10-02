package delivery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestToolReferenceRejectsInvalidMCPIdentity(t *testing.T) {
	for _, ref := range []protocol.ToolRef{
		{Type: protocol.ToolRefMCP, Server: "files", Name: "tool/name"},
		{Type: protocol.ToolRefMCP, Server: "Files", Name: "read"},
	} {
		if _, err := toolRefFromWire(ref); err == nil {
			t.Fatalf("invalid reference = %v", err)
		}
	}
}
