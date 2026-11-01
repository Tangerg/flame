package terminal

import (
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/oolong/components/headless"
	"strings"
	"testing"
)

func TestEndpointFieldExplainsTheEditedPathEvenWhenPopulated(t *testing.T) {
	field := &providerEndpointDraft{field: headless.NewText(headless.TextConfig{}), policy: flamemodel.EndpointPolicy{Example: "https://example.invalid/proxy", AppendsPath: "/v1"}}
	field.Set("https://example.invalid/proxy/v1")
	if !strings.Contains(field.field.Label, "/proxy/v1/v1") {
		t.Fatal(field.field.Label)
	}
	field.Set("https://example.invalid/team")
	if !strings.Contains(field.field.Label, "/team/v1") {
		t.Fatal(field.field.Label)
	}
}
