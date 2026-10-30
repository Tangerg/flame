package protocol

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestDiffRowPreservesBlankCodeAndRequiresPresence(t *testing.T) {
	for _, input := range []string{
		`{"type":"context","leftLine":1,"rightLine":1,"code":""}`,
		`{"type":"added","rightLine":1,"code":""}`,
		`{"type":"added","rightLine":1,"code":"", "noNewline":true}`,
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
		var fields map[string]jsontext.Value
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(input, "noNewline") && string(fields["noNewline"]) != "true" {
			t.Fatalf("missing newline fact was lost: %s", encoded)
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
		`{"type":"hunk","text":"@@ -1 +1 @@","noNewline":true}`,
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
