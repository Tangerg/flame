package testsupport

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
)

func CommandContract(t *testing.T, name, target string) jsonvalue.Value {
	t.Helper()
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "consumer.role", Composition: extension.Selection, Scope: "installation"}, extension.MethodDescriptor{Name: name, Kind: extension.Mutation, Target: target, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract.Identity()
}
