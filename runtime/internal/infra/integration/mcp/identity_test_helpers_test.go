package mcp

import "github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

func testMCPServerName(raw string) mcpserver.ServerName {
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		panic(err)
	}
	return name
}
