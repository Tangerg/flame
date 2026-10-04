package delivery

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestSecretInputProjectionPreservesOnlyPresence(t *testing.T) {
	for _, values := range []map[string]string{nil, {"secret": ""}, {"secret": "credential"}} {
		record := plugin.Record{Values: values, Selected: plugin.Release{Inputs: []plugin.Input{{ID: "secret", Secret: true}}}}
		projected := presentInstallation(record)
		_, configured := values["secret"]
		value, present := projected.Values["secret"]
		if present != configured || configured && value != "[REDACTED]" {
			t.Fatalf("secret projection changed configured presence: %t, %q", present, value)
		}
		if strings.Contains(value, "credential") || values["secret"] == "[REDACTED]" {
			t.Fatal("projection changed the secret owner or exposed its value")
		}
	}
}
