package plugin

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
)

func TestRequiredBindingsCannotBeRetargetedThroughInputOrSnapshot(t *testing.T) {
	contract, err := capability.New(capability.Config{Reference: "model", Composition: capability.Selection, Scope: "installation"}, capability.MethodDescriptor{Name: "model.read", Kind: capability.Query, Target: "installation", Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
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
