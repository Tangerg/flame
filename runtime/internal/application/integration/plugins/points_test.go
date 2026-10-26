package plugins

import (
	"encoding/json/jsontext"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestInstalledPointValuesRequireTheConsumerContractAndRetainExactBindings(t *testing.T) {
	contract, err := extension.AdmitPoint(extension.PointDescriptor{Protocol: extension.Protocol, Reference: "reviews.analyzers", Composition: extension.Collection, Scope: "installation", Optional: true, Ordering: extension.CanonicalKeys, Value: jsontext.Value(`{"type":"object","required":["title"],"properties":{"title":{"type":"string"}},"additionalProperties":false}`), Applicability: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonvalue.Parse([]byte(`{"title":"evidence"}`))
	if err != nil {
		t.Fatal(err)
	}
	makeSnapshot := func(name string, declaration plugin.Declaration) plugin.Snapshot {
		declaration.Name = name
		release, err := plugin.NewRelease(testsupport.Digest(name), declaration)
		if err != nil {
			t.Fatal(err)
		}
		installation, err := plugin.New(testsupport.InstallationID(t), name, release)
		if err != nil {
			t.Fatal(err)
		}
		if err := installation.Enable(release); err != nil {
			t.Fatal(err)
		}
		return plugin.Snapshot{Installation: installation, Selected: release}
	}
	consumer := makeSnapshot("reviews", plugin.Declaration{Points: []plugin.Point{{ID: "analyzers", Contract: contract.Identity()}}})
	producer := makeSnapshot("checklist", plugin.Declaration{PointValues: []plugin.PointValue{{ID: "evidence", Contract: contract.Identity(), Value: value}}})
	store := requirementInstallations{snapshots: []plugin.Snapshot{producer, consumer}}
	context := extension.PointContext{Target: extension.Target{Scope: "installation", Reference: consumer.Installation.ID().String()}, Attributes: jsontext.Value(`{}`)}
	bindings, err := ResolvePoint(t.Context(), store, contract, context, nil)
	if err != nil || len(bindings) != 1 || bindings[0].Reference.Installation != producer.Installation.ID() || bindings[0].Digest != producer.Selected.Digest() || bindings[0].Value != value {
		t.Fatalf("installed point binding: %+v, %v", bindings, err)
	}
	foreign := context
	foreign.Target.Reference = producer.Installation.ID().String()
	if _, err := ResolvePoint(t.Context(), store, contract, foreign, nil); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("another installation borrowed the consumer's point declaration: %v", err)
	}
	otherConsumer := makeSnapshot("other-reviews", plugin.Declaration{Points: []plugin.Point{{ID: "analyzers", Contract: contract.Identity()}}})
	consumer.Installation.Disable()
	store.snapshots = []plugin.Snapshot{producer, consumer, otherConsumer}
	if _, err := ResolvePoint(t.Context(), store, contract, context, nil); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("disabled target borrowed another consumer's declaration: %v", err)
	}
	if err := consumer.Installation.Enable(consumer.Selected); err != nil {
		t.Fatal(err)
	}
	store.snapshots = []plugin.Snapshot{producer}
	if _, err := ResolvePoint(t.Context(), store, contract, context, nil); err == nil {
		t.Fatal("producer manufactured an undeclared consumer point")
	}
	producer.Installation.Disable()
	store.snapshots = []plugin.Snapshot{producer, consumer}
	if bindings, err := ResolvePoint(t.Context(), store, contract, context, nil); err != nil || len(bindings) != 0 {
		t.Fatalf("disabled optional contributor: %+v, %v", bindings, err)
	}
}
