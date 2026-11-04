package delivery

import (
	"context"
	json "encoding/json/v2"
	"strings"
	"testing"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type inspectedPoint struct {
	pluginCallUseCases
	contract *plugincontract.Point
}

func (s *inspectedPoint) ResolvePoint(_ context.Context, contract *plugincontract.Point, _ plugincontract.PointContext, _ []plugins.PointReference) ([]plugins.PointBinding, error) {
	s.contract = contract
	return []plugins.PointBinding{}, nil
}

func TestPointProjectionAndQueryPreserveCanonicalNumericSchema(t *testing.T) {
	contract, err := plugincontract.AdmitPoint(plugincontract.PointDescriptor{
		Protocol: plugincontract.Protocol, Reference: "example.threshold", Scope: "workspace",
		Composition: plugincontract.Collection, Ordering: plugincontract.CanonicalKeys, Optional: true,
		Value: []byte(`{"type":"integer","maximum":18446744073709551615}`), Applicability: []byte(`true`),
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := framevalue.ParseJSON([]byte(`18446744073709551615`))
	if err != nil {
		t.Fatal(err)
	}
	release := testsupport.Release(t, "threshold", plugin.Declaration{
		Name:        "threshold",
		Points:      []plugin.Point{{ID: "threshold", Contract: contract.Identity()}},
		PointValues: []plugin.PointValue{{ID: "maximum", Contract: contract.Identity(), Value: value}},
	})
	projected, err := presentPluginRelease(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.ValidateWireTree(projected); err != nil {
		t.Fatal(err)
	}
	raw := string(contract.Identity().Bytes())
	if projected.Points[0].Contract != raw || projected.PointValues[0].Contract != raw || projected.PointValues[0].Value != "18446744073709551615" || !strings.Contains(raw, "18446744073709551615") {
		t.Fatal("point projection changed the admitted schema identity")
	}
	request := protocol.ResolvePluginPointRequest{Contract: projected.Points[0].Contract, Target: portable.Target{Scope: "workspace", Reference: "/project"}, Attributes: `{}`, Order: []protocol.PluginPointReference{}}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var received protocol.ResolvePluginPointRequest
	if err := json.Unmarshal(body, &received); err != nil {
		t.Fatal(err)
	}
	owner := &inspectedPoint{}
	if _, err := (&Handler{pluginCalls: owner}).ResolvePluginPoint(t.Context(), received); err != nil {
		t.Fatal(err)
	}
	if owner.contract == nil || owner.contract.Identity() != contract.Identity() {
		t.Fatal("point query did not reach its owner with the original contract")
	}
	if err := json.Unmarshal([]byte(`{"contract":`+raw+`,"target":{"scope":"workspace","reference":"/project"},"attributes":{},"order":[]}`), &received); err == nil {
		t.Fatal("former object-shaped point contract was admitted")
	}
}
