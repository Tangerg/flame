package dispatch

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPluginEnablementJSONCannotEraseAuthoredPresence(t *testing.T) {
	const id = "12345678-1234-1234-1234-123456789abc"
	for _, raw := range []string{
		`{"installationId":"` + id + `"}`,
		`{"installationId":"` + id + `","enabled":null}`,
	} {
		var request protocol.SetPluginEnablementRequest
		if err := decodeParams(jsontext.Value(raw), &request); err == nil {
			t.Fatalf("unbound enablement became disablement: %s", raw)
		}
	}
	var request protocol.SetPluginEnablementRequest
	if err := decodeParams(jsontext.Value(`{"installationId":"`+id+`","enabled":false}`), &request); err != nil || request.Enabled {
		t.Fatalf("explicit disablement: %+v, %v", request, err)
	}
}

func TestPluginInputChangePreservesEmptySetAndExplicitClear(t *testing.T) {
	for _, change := range []protocol.PluginValueChange{{Type: protocol.PluginValueSet, Value: new("")}, {Type: protocol.PluginValueClear}} {
		body, err := json.Marshal(change)
		if err != nil {
			t.Fatal(err)
		}
		var decoded protocol.PluginValueChange
		if err = json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		if err = protocol.ValidateWireTree(decoded); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if change.Type == protocol.PluginValueSet && (decoded.Value == nil || *decoded.Value != "") {
			t.Fatal("lost explicit empty input")
		}
		if change.Type == protocol.PluginValueClear && decoded.Value != nil {
			t.Fatal("clear introduced an input")
		}
	}
}

func TestPluginInputVariantErrorsSurviveRPCProjection(t *testing.T) {
	for _, test := range []struct {
		change string
		field  string
	}{
		{change: `{"type":"set"}`, field: `valueChanges["credential.name"].value`},
		{change: `{"type":"clear","value":"replacement"}`, field: `valueChanges["credential.name"].value`},
		{change: `{"type":"replace","value":"replacement"}`, field: `valueChanges["credential.name"].type`},
	} {
		raw := `{"installationId":"00000000-0000-4000-8000-000000000001","digest":"` + strings.Repeat("1", 64) + `","valueChanges":{"credential.name":` + test.change + `},"disabledServers":[],"disabledSkills":[]}`
		_, failure := decodeForTest[protocol.ConfigurePluginRequest](&transport.Request{Params: jsontext.Value(raw)})
		if failure == nil || failure.Code != codeInvalidParams {
			t.Fatalf("invalid input variant admitted: %s, %+v", test.change, failure)
		}
		var problem protocol.ProblemData
		if err := json.Unmarshal(failure.Data, &problem); err != nil {
			t.Fatal(err)
		}
		if len(problem.Errors) != 1 || problem.Errors[0].Field != test.field {
			t.Fatalf("input variant lost its field address: %+v", problem.Errors)
		}
	}
}
