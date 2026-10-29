package plugin

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
)

func TestRequiredBindingsCannotBeRetargetedThroughInputOrSnapshot(t *testing.T) {
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "model", Composition: portable.Selection, Scope: extension.ScopeInstallation}, portable.MethodDescriptor{Name: "model.read", Kind: portable.Query, Target: extension.ScopeInstallation, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	provider := Dependency{InstallationID: testInstallationID(t), Digest: testDigest("model")}
	values := []ResolvedRequirement{{Contribution: "consumer", ID: "model", Contract: contract.Identity(), Bindings: []BoundContribution{{Provider: provider, Contribution: "model", Server: serverName("backend"), Export: "model", Contract: contract.Identity()}}}}
	bindings, err := NewRequiredBindings(values)
	if err != nil {
		t.Fatal(err)
	}
	other := Dependency{InstallationID: testInstallationID(t), Digest: testDigest("replacement")}
	values[0].Bindings[0].Provider = other
	snapshot := bindings.Snapshot()
	snapshot[0].Bindings[0].Provider = other
	if !bindings.Allows(provider, "model", contract.Identity()) || bindings.Allows(other, "model", contract.Identity()) {
		t.Fatal("frozen dependency was retargeted")
	}
}
