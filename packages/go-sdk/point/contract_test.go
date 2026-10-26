package point

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
)

func testPoint(t *testing.T, composition capability.Composition, optional bool, ordering Ordering) *Contract {
	t.Helper()
	contract, err := Admit(Definition{Protocol: capability.Protocol, Reference: "reviews.analyzers", Composition: composition, Scope: "installation", Optional: optional, Ordering: ordering, Value: jsontext.Value(`{"type":"object","required":["title"],"properties":{"title":{"type":"string"}},"additionalProperties":false}`), Applicability: jsontext.Value(`{"type":"object","properties":{"attributes":{"type":"object","required":["active"],"properties":{"active":{"const":true}}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func testValue(t *testing.T, key string) Value {
	t.Helper()
	data, err := jsoninput.Encode(map[string]string{"title": key})
	if err != nil {
		t.Fatal(err)
	}
	return Value{Key: key, Data: data}
}

func TestPointOwnsSchemaApplicabilityAbsenceAndDetachedMetadata(t *testing.T) {
	contract := testPoint(t, capability.Collection, true, CanonicalKeys)
	definition := contract.Definition()
	definition.Value[0] = 'x'
	if err := contract.ValidateValue(testValue(t, "a").Data); err != nil {
		t.Fatal(err)
	}
	invalid, err := jsoninput.Parse([]byte(`{"title":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateValue(invalid); err == nil {
		t.Fatal("invalid consumer-defined value admitted")
	}
	context := Context{Target: capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}, Attributes: jsontext.Value(`{"active":true}`)}
	if applicable, err := contract.Applicable(context); err != nil || !applicable {
		t.Fatalf("applicability: %v, %v", applicable, err)
	}
	context.Attributes = jsontext.Value(`{"active":false}`)
	if applicable, err := contract.Applicable(context); err != nil || applicable {
		t.Fatalf("inapplicable context: %v, %v", applicable, err)
	}
	if _, err := testPoint(t, capability.Collection, false, CanonicalKeys).Compose(nil, nil); err == nil {
		t.Fatal("required absence was treated as success")
	}
}

func TestPointCompositionUsesCanonicalKeysOrExplicitConsumerOrder(t *testing.T) {
	values := []Value{testValue(t, "b"), testValue(t, "a")}
	collection := testPoint(t, capability.Collection, true, CanonicalKeys)
	composed, err := collection.Compose(values, nil)
	if err != nil || len(composed) != 2 || composed[0].Key != "a" {
		t.Fatalf("canonical collection: %+v, %v", composed, err)
	}
	if _, err := collection.Compose([]Value{values[0], values[0]}, nil); err == nil {
		t.Fatal("duplicate canonical contribution admitted")
	}
	selection := testPoint(t, capability.Selection, false, ExplicitList)
	if chosen, err := selection.Compose(values, []string{"b"}); err != nil || len(chosen) != 1 || chosen[0].Key != "b" {
		t.Fatalf("explicit selection: %+v, %v", chosen, err)
	}
	decoration := testPoint(t, capability.Decoration, true, ExplicitList)
	if ordered, err := decoration.Compose(values, []string{"b", "a"}); err != nil || ordered[0].Key != "b" {
		t.Fatalf("outermost-first order: %+v, %v", ordered, err)
	}
	for _, order := range [][]string{{"unknown"}, {"a", "a"}} {
		if _, err := decoration.Compose(values, order); err == nil {
			t.Fatal("invalid explicit order admitted")
		}
	}
	presentation := testPoint(t, capability.Collection, true, ExplicitList)
	if _, err := presentation.Compose(values, []string{"a"}); err == nil {
		t.Fatal("explicit collection order omitted an applicable member")
	}
}
