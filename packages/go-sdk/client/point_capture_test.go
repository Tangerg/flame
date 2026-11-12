package client_test

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/frame/portable"
)

type requiredPointValue struct {
	Name string `json:"name"`
}

type replacingPointResolver struct {
	contract    *plugincontract.Point
	replacement *plugincontract.Point
}

func (r replacingPointResolver) ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error) {
	*r.contract = *r.replacement
	return protocol.NewPage([]protocol.PluginPointBinding{{InstallationID: numericInstallation, Digest: identity.DigestStrings("point").String(), Contribution: "point", Value: `{}`}}), nil
}

func TestPointResolutionKeepsOriginalSchemaAcrossTheCall(t *testing.T) {
	schema, err := portable.SchemaFor[requiredPointValue]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := plugincontract.PointDescriptor{Protocol: plugincontract.Protocol, Reference: "point", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation, Ordering: plugincontract.CanonicalKeys, Value: schema, Applicability: []byte(`true`)}
	contract, err := plugincontract.AdmitPoint(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.Value = []byte(`true`)
	replacement, err := plugincontract.AdmitPoint(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	values, err := client.ResolvePointValues[requiredPointValue](t.Context(), replacingPointResolver{contract, replacement}, contract, portable.Target{Scope: extension.ScopeInstallation, Reference: numericInstallation}, struct{}{}, nil, protocol.CallOptions{})
	if err == nil || len(values) != 0 {
		t.Fatalf("replacement schema admitted a value missing the original required field: %+v, %v", values, err)
	}
}
