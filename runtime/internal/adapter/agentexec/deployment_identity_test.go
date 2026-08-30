package agentexec

import (
	"strings"
	"testing"
)

func TestDeploymentIdentityIsExactAndBounded(t *testing.T) {
	want := strings.Repeat("界", maximumDeploymentIdentityCharacters)
	identity, err := parseDeploymentIdentity("deployment identity", want)
	if err != nil {
		t.Fatalf("parse boundary: %v", err)
	}
	if identity.String() != want {
		t.Fatalf("exact identity changed: %q", identity.String())
	}

	invalid := []string{
		"",
		" deployment",
		"deployment ",
		"deploy\nment",
		"deploy\u200bment",
		string([]byte{0xff}),
		strings.Repeat("界", maximumDeploymentIdentityCharacters+1),
	}
	for _, text := range invalid {
		if _, err := parseDeploymentIdentity("deployment identity", text); err == nil {
			t.Errorf("parse accepted %q", text)
		}
	}
}
