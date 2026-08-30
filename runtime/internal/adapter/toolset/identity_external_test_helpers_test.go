package toolset_test

import "github.com/Tangerg/flame/runtime/internal/domain/mcpserver"

func testMCPServerName(raw string) mcpserver.ServerName {
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		panic(err)
	}
	return name
}
