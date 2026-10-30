package mcpserver

import "testing"

func TestAuthorityFingerprintDistinguishesArgumentsFromExecutionEnvironment(t *testing.T) {
	name, err := ParseServerName("tools")
	if err != nil {
		t.Fatal(err)
	}
	server := func(connection StdioConnection) Server {
		return Server{Source: UserSource(), Name: name, Connection: connection}
	}
	base := server(StdioConnection{Command: "node", Dir: "/srv"})
	split := server(StdioConnection{Command: "node", Dir: "/srv", Args: []string{"x", "y"}})
	joined := server(StdioConnection{Command: "node", Dir: "/srv", Args: []string{"x"}, ExecutionEnvironment: "y"})
	if split.AuthorityFingerprint() == joined.AuthorityFingerprint() {
		t.Fatal("Args [x y] and Args [x] with execution environment y share one authority fingerprint")
	}
	shifted := server(StdioConnection{Command: "node", Args: []string{"/srv"}})
	if base.AuthorityFingerprint() == shifted.AuthorityFingerprint() {
		t.Fatal("a directory and an argument with the same text share one authority fingerprint")
	}
}
