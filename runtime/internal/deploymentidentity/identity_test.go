package deploymentidentity

import (
	"strings"
	"testing"
)

func TestDeploymentIdentitiesAreExactBoundedAndDistinct(t *testing.T) {
	want := strings.Repeat("界", MaximumCharacters)
	implementation, err := ParseImplementation(want)
	if err != nil {
		t.Fatalf("ParseImplementation boundary: %v", err)
	}
	configuration, err := ParseConfiguration(want)
	if err != nil {
		t.Fatalf("ParseConfiguration boundary: %v", err)
	}
	if implementation.String() != want || configuration.String() != want {
		t.Fatalf("exact identities changed: %q/%q", implementation.String(), configuration.String())
	}

	invalid := []string{
		"",
		" deployment",
		"deployment ",
		"deploy\nment",
		"deploy\u200bment",
		string([]byte{0xff}),
		strings.Repeat("界", MaximumCharacters+1),
	}
	for _, text := range invalid {
		if _, err := ParseImplementation(text); err == nil {
			t.Errorf("ParseImplementation accepted %q", text)
		}
		if _, err := ParseConfiguration(text); err == nil {
			t.Errorf("ParseConfiguration accepted %q", text)
		}
	}
}
