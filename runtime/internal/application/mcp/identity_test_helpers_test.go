package mcp

import "github.com/Tangerg/flame/runtime/internal/domain/mcpserver"

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
