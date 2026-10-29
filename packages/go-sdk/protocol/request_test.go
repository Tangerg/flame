package protocol_test

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestRequestDecodingPreservesOpaqueEvidence(t *testing.T) {
	const raw = `{"name":"read","arguments":{"identity":9007199254740993,"limit":1e1000,"value":null}}`
	var request protocol.InvokeToolRequest
	if err := protocol.DecodeRequest([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{"9007199254740993", "1e1000", `"value":null`} {
		if !strings.Contains(string(encoded), evidence) {
			t.Fatalf("authored request lost %s: %s", evidence, encoded)
		}
	}
}

func TestInvalidRequestDecodingReturnsNoPartialCommand(t *testing.T) {
	for _, raw := range []string{
		`{"source":"/package","unknown":true}`,
		`{"source":"/package","source":"/replacement"}`,
		`{"source":"/package"} {}`,
		`{"source":"\ud800"}`,
		`{"source":null}`,
		`{"source":""}`,
	} {
		var request protocol.InstallPluginRequest
		err := protocol.DecodeRequest([]byte(raw), &request)
		if !errors.Is(err, protocol.ErrInvalidParams) || !reflect.DeepEqual(request, protocol.InstallPluginRequest{}) {
			t.Fatalf("invalid authored request returned a partial command: %+v, %v", request, err)
		}
	}
}

func TestRequestDecodingPreservesTargetOnFailure(t *testing.T) {
	request := protocol.InstallPluginRequest{Source: "/original"}
	err := protocol.DecodeRequest([]byte(`{"source":"/replacement","unknown":true}`), &request)
	if !errors.Is(err, protocol.ErrInvalidParams) || request.Source != "/original" {
		t.Fatalf("invalid request changed the destination: %+v, %v", request, err)
	}
}

func TestRequestDecodingProjectsRootShapeFailuresAsInvalidParameters(t *testing.T) {
	for _, target := range []any{&protocol.RequestMeta{}, &protocol.StartRunRequest{}} {
		err := protocol.DecodeRequest([]byte(`null`), target)
		if !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("root null was not rejected: %T, %v", target, err)
		}
		problem := protocol.ProjectError(err).Problem()
		if problem.Type != protocol.ErrInvalidParams.Error() || !reflect.DeepEqual(problem.Errors, []protocol.FieldError{{Field: "$", Detail: "must not be null"}}) {
			t.Fatalf("root shape failure lost its invalid-parameters evidence: %+v", problem)
		}
		if err := protocol.ValidateWireTree(problem); err != nil {
			t.Fatalf("root shape failure produced an invalid response: %v", err)
		}
	}
}

func TestRequestDecodingReportsAuthoredFieldPresence(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	for _, test := range []struct {
		raw    string
		field  string
		detail string
	}{
		{raw: `{"installationId":"` + id + `"}`, field: "enabled", detail: "is required"},
		{raw: `{"installationId":"` + id + `","enabled":null}`, field: "enabled", detail: "must not be null"},
	} {
		var request protocol.SetPluginEnablementRequest
		err := protocol.DecodeRequest([]byte(test.raw), &request)
		constraint, ok := errors.AsType[*protocol.ConstraintError](err)
		if !errors.Is(err, protocol.ErrInvalidParams) || !ok || !reflect.DeepEqual(constraint.Fields, []protocol.FieldError{{Field: test.field, Detail: test.detail}}) {
			t.Fatalf("authored field error was lost: %v", err)
		}
	}
}

func TestRequestDecodingRejectsInvalidTargets(t *testing.T) {
	for _, target := range []any{nil, (*protocol.InstallPluginRequest)(nil), protocol.InstallPluginRequest{}} {
		if err := protocol.DecodeRequest([]byte(`{"source":"/package"}`), target); !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("invalid request destination accepted: %T, %v", target, err)
		}
	}
}

func TestRequestDecodingValidatesTypedMapChanges(t *testing.T) {
	requestJSON := func(change string) string {
		return `{"installationId":"00000000-0000-4000-8000-000000000001","digest":"` + strings.Repeat("1", 64) + `","valueChanges":{"credential":` + change + `},"serverChanges":{},"skillChanges":{}}`
	}
	for _, test := range []struct {
		change string
		want   protocol.PluginValueChange
	}{
		{`{"type":"set","value":"replacement"}`, protocol.PluginValueChange{Type: protocol.PluginValueSet, Value: new("replacement")}},
		{`{"type":"set","value":""}`, protocol.PluginValueChange{Type: protocol.PluginValueSet, Value: new("")}},
		{`{"type":"clear"}`, protocol.PluginValueChange{Type: protocol.PluginValueClear}},
	} {
		var request protocol.ConfigurePluginRequest
		if err := protocol.DecodeRequest([]byte(requestJSON(test.change)), &request); err != nil {
			t.Fatalf("valid map change fixture rejected: %s, %v", test.change, err)
		}
		if !reflect.DeepEqual(request.ValueChanges, map[string]protocol.PluginValueChange{"credential": test.want}) {
			t.Fatalf("valid map change disappeared: %+v", request)
		}
	}
	for _, test := range []struct{ change, field, detail string }{
		{`{"type":"set"}`, `valueChanges["credential"].value`, "is required"},
		{`{"type":"clear","value":"replacement"}`, `valueChanges["credential"].value`, "must not be present here"},
		{`{"type":"replace","value":"replacement"}`, `valueChanges["credential"].type`, "must be one of set, clear"},
	} {
		t.Run(test.change, func(t *testing.T) {
			var request protocol.ConfigurePluginRequest
			err := protocol.DecodeRequest([]byte(requestJSON(test.change)), &request)
			constraint, ok := errors.AsType[*protocol.ConstraintError](err)
			want := []protocol.FieldError{{Field: test.field, Detail: test.detail}}
			if !errors.Is(err, protocol.ErrInvalidParams) || !ok || constraint.Shape != "ConfigurePluginRequest" || !reflect.DeepEqual(constraint.Fields, want) || !reflect.DeepEqual(request, protocol.ConfigurePluginRequest{}) {
				t.Fatalf("map change refusal = %+v, %v; want fields %+v and no partial command", request, err, want)
			}
		})
	}
}
