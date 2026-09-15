package protocol

import (
	"encoding/json"
	"testing"
)

func TestDiffRowPreservesBlankCodeAndRequiresPresence(t *testing.T) {
	for _, input := range []string{
		`{"type":"context","leftLine":1,"rightLine":1,"code":""}`,
		`{"type":"added","rightLine":1,"code":""}`,
		`{"type":"deleted","leftLine":1,"code":""}`,
	} {
		var row DiffRow
		if err := json.Unmarshal([]byte(input), &row); err != nil {
			t.Fatal(err)
		}
		if err := ValidateWireTree(row); err != nil {
			t.Fatalf("blank row %s: %v", input, err)
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["code"]) != `""` {
			t.Fatalf("blank code was lost: %s", encoded)
		}
	}
	for _, input := range []string{
		`{"type":"context","leftLine":1,"rightLine":1}`,
		`{"type":"added","rightLine":1,"code":null}`,
		`{"type":"deleted","leftLine":1}`,
		`{"type":"hunk","text":"@@ -1 +1 @@","code":""}`,
	} {
		var row DiffRow
		if err := json.Unmarshal([]byte(input), &row); err != nil {
			t.Fatal(err)
		}
		if err := ValidateWireTree(row); err == nil {
			t.Fatalf("invalid presence accepted: %s", input)
		}
	}
}
