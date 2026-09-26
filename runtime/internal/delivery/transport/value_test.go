package transport

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestTypedWireDecoderPreservesOpaqueNumericEvidence(t *testing.T) {
	const raw = `{"name":"tool","arguments":{"identity":9007199254740993,"limit":1e1000,"value":null}}`
	var value protocol.ToolInvocation
	if err := DecodeValue(jsontext.Value(raw), &value, "result"); err != nil {
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

func TestRemoteRequiredFieldsUsePublishedShape(t *testing.T) {
	for _, test := range []struct {
		raw   string
		value any
	}{
		{raw: `{}`, value: protocol.SessionSnapshot{}},
		{raw: `{"name":"tool"}`, value: protocol.ToolInvocation{}},
		{raw: `{"items":[],"runs":[],"interrupts":[],"plan":{"sessionId":"ses_test","state":{}}}`, value: protocol.SessionSnapshot{}},
	} {
		if err := ValidateRequiredFields(jsontext.Value(test.raw), reflect.TypeOf(test.value), "result"); err == nil {
			t.Fatalf("missing required response field accepted: %s", test.raw)
		}
	}
	if err := ValidateRequiredFields(jsontext.Value(`{"items":[],"runs":[],"interrupts":[]}`), reflect.TypeFor[protocol.SessionSnapshot](), "result"); err != nil {
		t.Fatalf("optional response fields became required: %v", err)
	}
}
