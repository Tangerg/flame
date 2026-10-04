package contractshape_test

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestTypedWireDecoderPreservesOpaqueNumericEvidence(t *testing.T) {
	const raw = `{"name":"tool","arguments":{"identity":9007199254740993,"limit":1e1000,"value":null}}`
	var value protocol.ToolInvocation
	if err := contractshape.DecodeValue(jsontext.Value(raw), &value, "result"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{"9007199254740993", "1e1000", `"value":null`} {
		if !strings.Contains(string(encoded), evidence) {
			t.Fatalf("wire translation lost %s: %s", evidence, encoded)
		}
	}
}

func TestTypedWireRequiredFieldsUsePublishedShape(t *testing.T) {
	for _, test := range []struct {
		raw   string
		value any
	}{
		{raw: `{}`, value: protocol.SessionSnapshot{}},
		{raw: `{"name":"tool"}`, value: protocol.ToolInvocation{}},
		{raw: `{"items":[],"runs":[],"interrupts":[],"plan":{"sessionId":"ses_test","state":{}}}`, value: protocol.SessionSnapshot{}},
	} {
		if err := contractshape.DecodeValue(jsontext.Value(test.raw), reflect.New(reflect.TypeOf(test.value)).Interface(), "result"); err == nil {
			t.Fatalf("missing required response field accepted: %s", test.raw)
		}
	}
	var snapshot protocol.SessionSnapshot
	if err := contractshape.DecodeValue(jsontext.Value(`{"items":[],"runs":[],"interrupts":[]}`), &snapshot, "result"); err != nil {
		t.Fatalf("optional response fields became required: %v", err)
	}
}
