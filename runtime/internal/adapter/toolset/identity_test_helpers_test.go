package toolset

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	identitytool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testMCPRef(server mcpserver.ServerName, remote mcpserver.RemoteToolName) identitytool.Ref {
	return testsupport.MCPTool(testsupport.UserMCPServer(server.String()), remote.String())
}
