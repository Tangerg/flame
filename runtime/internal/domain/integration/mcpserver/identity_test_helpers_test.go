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
