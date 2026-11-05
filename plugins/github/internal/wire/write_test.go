package wire

import (
	"strings"
	"testing"

	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/scope/core/jsonschema"
)

func TestWriteSchemasPreserveOwnedConstraints(t *testing.T) {
	for _, test := range []struct {
		name    string
		schema  func() ([]byte, error)
		valid   string
		invalid string
	}{
		{
			name: "confirmed deletion", schema: portable.SchemaFor[DeletedComment],
			valid: `{"id":1,"deleted":true}`, invalid: `{"id":1,"deleted":false}`,
		},
		{
			name: "confirmed merge", schema: portable.SchemaFor[MergeResult],
			valid: `{"sha":"abc","merged":true,"message":"merged"}`, invalid: `{"sha":"abc","merged":false,"message":"merged"}`,
		},
		{
			name: "inspected commit", schema: portable.SchemaFor[SubmitReviewInput],
			valid:   `{"repository":"owner/repo","number":1,"commitId":"` + strings.Repeat("a", 40) + `","event":"APPROVE"}`,
			invalid: `{"repository":"owner/repo","number":1,"commitId":"latest","event":"APPROVE"}`,
		},
		{
			name: "bounded title", schema: portable.SchemaFor[CreateIssueInput],
			valid:   `{"repository":"owner/repo","title":"` + strings.Repeat("x", MaxTitleRunes) + `"}`,
			invalid: `{"repository":"owner/repo","title":"` + strings.Repeat("x", MaxTitleRunes+1) + `"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := test.schema()
			if err != nil {
				t.Fatal(err)
			}
			schema, err := jsonschema.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate([]byte(test.valid)); err != nil {
				t.Fatalf("valid value refused: %v", err)
			}
			if err := schema.Validate([]byte(test.invalid)); err == nil {
				t.Fatal("invalid value bypassed the published constraint")
			}
		})
	}
}
