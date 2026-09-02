package strictjson

import (
	"strings"
	"testing"
)

func TestValidateUniqueMembersRejectsAmbiguousJSONAtEveryDepth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "root", json: `{"name":"first","name":"second"}`, want: `duplicate JSON member "name" at $`},
		{name: "escaped", json: `{"name":"first","\u006eame":"second"}`, want: `duplicate JSON member "name" at $`},
		{name: "nested", json: `{"items":[{"name":"first","name":"second"}]}`, want: `duplicate JSON member "name" at $.items[0]`},
		{name: "trailing", json: `{} {}`, want: "JSON contains more than one value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateUniqueMembers([]byte(test.json))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateUniqueMembers error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateUniqueMembersAcceptsOneUnambiguousValue(t *testing.T) {
	t.Parallel()
	if err := ValidateUniqueMembers([]byte(`{"items":[{"name":"first"}],"enabled":true}`)); err != nil {
		t.Fatalf("ValidateUniqueMembers: %v", err)
	}
}
