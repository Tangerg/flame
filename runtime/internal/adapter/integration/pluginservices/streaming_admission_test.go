package pluginservices

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestUnaryInstalledAdmissionRefusesStreamBeforeAccessingRouteOwners(t *testing.T) {
	method, err := qimumcp.NewStreamingMethod[struct{}, string, bool](qimumcp.MethodConfig{Name: "records.stream", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Stream installed records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "records", Scope: extension.ScopeInstallation, Composition: portable.Selection}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	caller := &CommandCalls{}
	_, err = caller.Prepare(t.Context(), plugins.CallRequest{Installation: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), Contribution: "records", Contract: contract, Method: "records.stream"})
	if !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("stream reached unary routing: %v", err)
	}
}
