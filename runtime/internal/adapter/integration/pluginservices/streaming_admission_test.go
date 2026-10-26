package pluginservices

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestUnaryInstalledAdmissionRefusesStreamBeforeAccessingRouteOwners(t *testing.T) {
	method, err := pluginsdk.NewStreamingMethod[struct{}, string, bool](pluginsdk.MethodConfig{Name: "records.stream", Kind: extension.Mutation, Target: "installation", Description: "Stream installed records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "records", Scope: "installation", Composition: extension.Selection}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	caller := &CommandCalls{}
	_, err = caller.Prepare(t.Context(), plugins.CallRequest{Installation: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), Contribution: "records", Contract: contract, Method: "records.stream"})
	if !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("stream reached unary routing: %v", err)
	}
}
