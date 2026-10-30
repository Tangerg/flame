package mcpserver

import "testing"

func TestAuthorityFingerprintDistinguishesArgumentsFromExecutionEnvironment(t *testing.T) {
	name, err := ParseServerName("tools")
	if err != nil {
		t.Fatal(err)
	}
	base := Server{Source: UserSource(), Name: name, Transport: TransportStdio, Command: "node", Dir: "/srv"}
	split := base
	split.Args = []string{"x", "y"}
	joined := base
	joined.Args = []string{"x"}
	joined.ExecutionEnvironment = "y"
	if split.AuthorityFingerprint() == joined.AuthorityFingerprint() {
		t.Fatal("Args [x y] and Args [x] with execution environment y share one authority fingerprint")
	}
	shifted := base
	shifted.Command, shifted.Dir, shifted.Args = "node", "", []string{"/srv"}
	if base.AuthorityFingerprint() == shifted.AuthorityFingerprint() {
		t.Fatal("a directory and an argument with the same text share one authority fingerprint")
	}
}
