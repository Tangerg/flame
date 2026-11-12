package contract_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
)

func TestPortableServiceProjectionCannotReplacePublishedCapability(t *testing.T) {
	method := portable.MethodDescriptor{Name: "notes.read", Description: "Read notes.", Kind: portable.Query, Target: extension.ScopeInstallation, Input: []byte(`{"type":"object"}`), Output: []byte(`true`)}
	capability, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "notes", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, method)
	if err != nil {
		t.Fatal(err)
	}
	identity, descriptor := capability.Identity(), capability.Descriptor()
	service := capability.Service()
	original := service.Identity()
	method.Name = "replacement.read"
	replacement, err := portable.NewCapability("replacement", method)
	if err != nil {
		t.Fatal(err)
	}
	*service = *replacement
	if capability.Identity() != identity || !reflect.DeepEqual(capability.Descriptor(), descriptor) || capability.Service().Identity() != original {
		t.Fatal("replacing a service projection changed the published contract")
	}
	if _, found := capability.Method("notes.read"); !found {
		t.Fatal("original callable disappeared behind its published identity")
	}
	if _, found := capability.Method("replacement.read"); found {
		t.Fatal("a projected service installed another callable behind the original identity")
	}
}
