package protocol_test

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
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
	for _, change := range []string{`{"type":"set"}`, `{"type":"clear","value":"replacement"}`, `{"type":"replace","value":"replacement"}`} {
		var request protocol.ConfigurePluginRequest
		raw := `{"installationId":"00000000-0000-4000-8000-000000000001","digest":"` + strings.Repeat("1", 64) + `","valueChanges":{"credential":` + change + `},"disabledServers":[],"disabledSkills":[]}`
		if err := protocol.DecodeRequest([]byte(raw), &request); !errors.Is(err, protocol.ErrInvalidParams) || !reflect.DeepEqual(request, protocol.ConfigurePluginRequest{}) {
			t.Fatalf("invalid map change became a command: %s, %+v, %v", change, request, err)
		}
	}
}
