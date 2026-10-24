package mcpserver

import "testing"

func TestExecutionEnvironmentIsCapturedAuthority(t *testing.T) {
	original := Server{Source: UserSource(), Transport: TransportStdio, Command: "/distribution/node", ExecutionEnvironment: "original-artifact-and-policy"}
	replaced := original
	replaced.ExecutionEnvironment = "different-artifact-or-policy"
	if original.AuthorityFingerprint() == replaced.AuthorityFingerprint() || original.SameConnection(replaced) {
		t.Fatal("execution environment replacement retained old activation authority")
	}
	replaced = original
	replaced.Env = map[string]string{"TOKEN": "rotated"}
	if original.AuthorityFingerprint() != replaced.AuthorityFingerprint() {
		t.Fatal("credential changed execution authority")
	}
}
