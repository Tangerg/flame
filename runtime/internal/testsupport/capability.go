package testsupport

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
)

func CommandContract(t *testing.T, name, target string) jsonvalue.Value {
	t.Helper()
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "consumer.role", Composition: portable.Selection, Scope: "installation"}, portable.MethodDescriptor{Name: name, Kind: portable.Mutation, Target: target, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract.Identity()
}
