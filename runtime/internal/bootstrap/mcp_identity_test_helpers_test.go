package bootstrap

import "github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

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

func testServerToolPolicy(disabled, autoApproved []string) mcpserver.ServerToolPolicy {
	parse := func(values []string) []mcpserver.RemoteToolName {
		names := make([]mcpserver.RemoteToolName, len(values))
		for i, value := range values {
			names[i] = testRemoteToolName(value)
		}
		return names
	}
	policy, err := mcpserver.NewServerToolPolicy(parse(disabled), parse(autoApproved))
	if err != nil {
		panic(err)
	}
	return policy
}
