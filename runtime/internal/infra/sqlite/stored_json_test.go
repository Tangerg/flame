package sqlite

import (
	"encoding/json"
	"testing"
)

func TestStoredJSONRejectsAmbiguousOrCorruptValues(t *testing.T) {
	type record struct {
		Name   string          `json:"name"`
		Nested json.RawMessage `json:"nested"`
	}
	for _, encoded := range []string{
		`{"name":"first","name":"second"}`,
		`{"name":"first","na\u006de":"second"}`,
		`{"nested":{"a":1,"a":2}}`,
		`{"nested":[{"a":1,"a":2}]}`,
		`{"name":"known","future":true}`,
		`{"Name":"wrong spelling"}`,
		`{"name":"first"}{"name":"second"}`,
		"{\"name\":\"\xff\"}",
		`{"name":`,
	} {
		t.Run(encoded, func(t *testing.T) {
			var out record
			if err := decodeStoredJSON([]byte(encoded), &out); err == nil {
				t.Fatal("corrupt stored JSON accepted")
			}
		})
	}
	var out record
	if err := decodeStoredJSON([]byte(` {"name":"valid","nested":{"a":[1,true,null]}} `), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "valid" || string(out.Nested) != `{"a":[1,true,null]}` {
		t.Fatalf("decoded record = %+v", out)
	}
}
