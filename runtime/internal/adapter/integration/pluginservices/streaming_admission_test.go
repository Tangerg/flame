package pluginservices

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/frame"
)

func TestUnaryInstalledAdmissionRefusesStreamBeforeAccessingRouteOwners(t *testing.T) {
	method, err := pluginmcp.NewStreamingMethod[struct{}, string, bool](frame.MethodConfig{Name: "records.stream", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Stream installed records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "records", Scope: extension.ScopeInstallation, Composition: plugincontract.Selection}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	caller := &CommandCalls{}
	_, err = caller.Prepare(t.Context(), plugins.CallRequest{Installation: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), Contribution: "records", Contract: contract, Method: "records.stream"})
	if !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("stream reached unary routing: %v", err)
	}
}
