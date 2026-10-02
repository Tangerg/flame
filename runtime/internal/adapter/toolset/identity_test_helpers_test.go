package toolset

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	identitytool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func testMCPServerName(raw string) mcpserver.ServerName {
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func testRemoteToolName(raw string) mcpserver.RemoteToolName {
	name, err := mcpserver.ParseRemoteToolName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func testMCPRef(server mcpserver.ServerName, remote mcpserver.RemoteToolName) identitytool.Ref {
	ref, err := identitytool.MCP(server, remote)
	if err != nil {
		panic(err)
	}
	return ref
}
