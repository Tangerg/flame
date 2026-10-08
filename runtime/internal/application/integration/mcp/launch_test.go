package mcp

import (
	"fmt"
	"strings"
	"testing"
)

func TestLaunchProjectionRedactsExecutionCredentials(t *testing.T) {
	secret := "private-execution-credential"
	input := Launch{Stdio: &Stdio{Command: "server", Env: map[string]string{"TOKEN": secret}}}
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		if output := fmt.Sprintf(verb, input); strings.Contains(output, secret) {
			t.Fatalf("launch projection exposed credentials: %s", output)
		}
	}
}
