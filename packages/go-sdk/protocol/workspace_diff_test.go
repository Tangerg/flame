package protocol_test

import (
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestDiffProjectionsRequireTheirOwnCompletePayload(t *testing.T) {
	const baseline = `"baseline":{"type":"emptyTree"}`
	const budget = `"budget":{"files":5000,"rows":5000,"bytes":67108864}`
	for _, payload := range []string{
		`"type":"raw","patch":""`,
		`"type":"rows","rows":{"files":[],"truncated":false,` + budget + `}`,
		`"type":"visual","visual":{"patch":"","manifest":[],"truncated":false,` + budget + `}`,
		`"type":"visual","visual":{"patch":"","manifest":[],"truncated":true,` + budget + `}`,
	} {
		var value protocol.Diff
		input := []byte(`{` + baseline + `,` + payload + `}`)
		if err := protocol.DecodeWire(input, &value); err != nil {
			t.Fatalf("decode %s: %v", input, err)
		}
		if err := protocol.ValidateWireTree(value); err != nil {
			t.Fatalf("validate %s: %v", input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) == "" {
			t.Fatalf("encode %s: %s, %v", input, encoded, err)
		}
		var roundtrip protocol.Diff
		if err := protocol.DecodeWire(encoded, &roundtrip); err != nil {
			t.Fatalf("round trip %s: %v", encoded, err)
		}
		if value.Type == protocol.DiffFormatRaw && (roundtrip.Patch == nil || *roundtrip.Patch != "") {
			t.Fatalf("empty complete raw patch was lost: %+v", roundtrip)
		}
	}
	for _, payload := range []string{
		`"files":[],"truncated":false`,
		`"type":"raw"`,
		`"type":"raw","patch":null`,
		`"type":"rows","patch":""`,
		`"type":"visual","patch":""`,
		`"type":"visual","visual":{"patch":"","truncated":false,` + budget + `}`,
		`"type":"visual","visual":{"patch":"","manifest":[],` + budget + `}`,
		`"type":"visual","visual":{"patch":"","manifest":[],"truncated":false}`,
		`"type":"visual","visual":{"patch":"","manifest":[],"truncated":false,"budget":{"files":0,"rows":5000,"bytes":67108864}}`,
		`"type":"raw","patch":"","rows":{"files":[],"truncated":false,` + budget + `}`,
		`"type":"visual","visual":{"patch":"","manifest":[],"truncated":false,` + budget + `},"patch":""`,
	} {
		input := []byte(`{` + baseline + `,` + payload + `}`)
		var value protocol.Diff
		if err := protocol.DecodeWire(input, &value); err == nil && protocol.ValidateWireTree(value) == nil {
			t.Fatalf("incomplete or mixed projection accepted: %s", input)
		}
	}
}
