package testsupport

import (
	"encoding/json/jsontext"
	"testing"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func CommandContract(t *testing.T, name, target string) framevalue.JSON {
	t.Helper()
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "consumer.role", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, portable.MethodDescriptor{Name: name, Kind: portable.Command, Target: target, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract.Identity()
}
