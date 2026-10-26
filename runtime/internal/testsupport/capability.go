package testsupport

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
)

func CommandContract(t *testing.T, name, target string) jsoninput.Input {
	t.Helper()
	contract, err := capability.New(capability.Config{Reference: "consumer.role", Composition: capability.Selection, Scope: "installation"}, capability.MethodDescriptor{Name: name, Kind: capability.Mutation, Target: target, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract.Identity()
}
