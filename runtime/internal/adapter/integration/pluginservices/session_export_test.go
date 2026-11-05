package pluginservices

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func TestRenderedSessionDocumentRequiresCompleteOriginalOutput(t *testing.T) {
	type event = pluginmcp.StreamValue[jsontext.Value, jsontext.Value]
	chunk := jsontext.Value(`"doc"`)
	complete, err := json.Marshal(pluginsdk.IdentifyDocument([]byte("doc")))
	if err != nil {
		t.Fatal(err)
	}
	good := jsontext.Value(complete)
	other, err := json.Marshal(pluginsdk.IdentifyDocument([]byte("different")))
	if err != nil {
		t.Fatal(err)
	}
	wrong := jsontext.Value(other)
	disconnected := errors.New("disconnected")
	for _, test := range []struct {
		name   string
		events []event
		err    error
		ok     bool
	}{
		{"complete", []event{{Chunk: &chunk}, {Result: &good}}, nil, true},
		{"missing final", []event{{Chunk: &chunk}}, nil, false},
		{"disconnected", []event{{Chunk: &chunk}}, disconnected, false},
		{"changed final", []event{{Chunk: &chunk}, {Result: &wrong}}, nil, false},
		{"after final", []event{{Chunk: &chunk}, {Result: &good}, {Chunk: &chunk}}, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := readRenderedDocument(func(yield func(event, error) bool) {
				for _, e := range test.events {
					if !yield(e, nil) {
						return
					}
				}
				if test.err != nil {
					yield(event{}, test.err)
				}
			})
			if test.ok {
				if err != nil || body != "doc" {
					t.Fatalf("result=%q error=%v", body, err)
				}
				return
			}
			if body != "" || err == nil {
				t.Fatalf("exposed partial output %q, %v", body, err)
			}
		})
	}
}
