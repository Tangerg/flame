package builtin

import (
	json "encoding/json/v2"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestNamesAreClosedAndDetached(t *testing.T) {
	names := Names()
	for _, name := range names {
		if !name.Valid() {
			t.Fatalf("declared name %q is invalid", name)
		}
	}
	names[0] = "not_a_builtin"
	if names[0].Valid() || Name("not_a_builtin").Valid() || Names()[0] == names[0] {
		t.Fatal("a consumer changed the canonical vocabulary")
	}
}

func TestNormalizedResultEncodingRetainsPresenceAndZeroCounts(t *testing.T) {
	zero := 0
	encoded, err := json.Marshal(CommandResult{ExitCode: &zero})
	if err != nil || !strings.Contains(string(encoded), `"exitCode":0`) {
		t.Fatalf("finished command = %s, %v", encoded, err)
	}
	encoded, err = json.Marshal(CommandResult{})
	if err != nil || strings.Contains(string(encoded), "exitCode") {
		t.Fatalf("unfinished command = %s, %v", encoded, err)
	}
	encoded, err = json.Marshal(PatchResult{Changes: []AppliedChange{{Path: "next.go", Status: Moved, From: "old.go"}}})
	if err != nil || !strings.Contains(string(encoded), `"added":0`) || !strings.Contains(string(encoded), `"removed":0`) {
		t.Fatalf("pure move = %s, %v", encoded, err)
	}
}

func TestShellSchemaStatesTheAutoBackgroundDefault(t *testing.T) {
	field, _ := reflect.TypeFor[ShellArguments]().FieldByName("AutoBackgroundAfterSeconds")
	want := "Defaults to " + strconv.Itoa(ShellAutoBackgroundSeconds) + "."
	if !strings.Contains(field.Tag.Get("jsonschema_description"), want) {
		t.Fatalf("auto-background description does not state %q", want)
	}
}

func TestEveryBuiltinDeclaresItsPresentation(t *testing.T) {
	for _, name := range Names() {
		if !slices.Contains(name.Category().Values(), string(name.Category())) {
			t.Errorf("%s has no presentation category", name)
		}
		if shape := name.ResultShape(); shape != "" && !shape.Valid() {
			t.Errorf("%s declares unknown result shape %q", name, shape)
		}
	}
	if len(presentations) != len(Names()) {
		t.Fatal("presentation table names a tool outside the built-in set")
	}
}
