package mcpserver

func testMCPServerName(raw string) ServerName {
	name, err := ParseServerName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func testRemoteToolName(raw string) RemoteToolName {
	name, err := ParseRemoteToolName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func testServerToolPolicy(disabled, autoApproved []string) ServerToolPolicy {
	toNames := func(raw []string) []RemoteToolName {
		names := make([]RemoteToolName, len(raw))
		for i, value := range raw {
			names[i] = testRemoteToolName(value)
		}
		return names
	}
	policy, err := NewServerToolPolicy(toNames(disabled), toNames(autoApproved))
	if err != nil {
		panic(err)
	}
	return policy
}
