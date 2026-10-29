package plugin

import (
	"encoding/json/jsontext"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
)

func TestContributionsHaveIndependentIDsAndCanShareOneBackend(t *testing.T) {
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "consumer.role", Composition: portable.Selection, Scope: extension.ScopeInstallation}, portable.MethodDescriptor{Name: "owner.read", Kind: portable.Query, Target: extension.ScopeInstallation, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	declaration := Declaration{Name: "contributions", Servers: []Server{{Name: serverName("backend"), Transport: stdio, Command: "backend"}}, Contributions: []Contribution{{ID: "first-provider", Server: serverName("backend"), Export: "first", Contract: contract.Identity()}, {ID: "second-provider", Server: serverName("backend"), Export: "second", Contract: contract.Identity()}}}
	release, err := NewRelease(testDigest("contributions"), declaration)
	if err != nil {
		t.Fatal(err)
	}
	for _, declared := range declaration.Contributions {
		if admitted, found := release.Contribution(declared.ID); !found || !reflect.DeepEqual(admitted, declared) {
			t.Fatalf("contribution lost its independent identity: %+v", declared)
		}
	}
	declaration.Contributions[1].ID = declaration.Contributions[0].ID
	if _, err := NewRelease(testDigest("duplicate"), declaration); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate contribution admission=%v", err)
	}
}
