package testsupport

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
)

func CommandContract(t *testing.T, name, target string) framevalue.JSON {
	t.Helper()
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "consumer.role", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, frame.MethodDescriptor{Name: name, Kind: frame.Command, Target: target, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract.Identity()
}
