package mcpserver

import "testing"

func TestExecutionEnvironmentIsCapturedAuthority(t *testing.T) {
	process := StdioConnection{Command: "/distribution/node", ExecutionEnvironment: "original-artifact-and-policy"}
	original := Server{Source: UserSource(), Connection: process}
	replaced := original
	replacedProcess := process
	replacedProcess.ExecutionEnvironment = "different-artifact-or-policy"
	replaced.Connection = replacedProcess
	if original.AuthorityFingerprint() == replaced.AuthorityFingerprint() || original.SameConnection(replaced) {
		t.Fatal("execution environment replacement retained old activation authority")
	}
	rotated := process
	rotated.Env = map[string]string{"TOKEN": "rotated"}
	replaced.Connection = rotated
	if original.AuthorityFingerprint() != replaced.AuthorityFingerprint() {
		t.Fatal("credential changed execution authority")
	}
}
