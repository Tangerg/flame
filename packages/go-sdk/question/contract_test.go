package question

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

func TestContractAnswerMatrix(t *testing.T) {
	for _, test := range []struct {
		name   string
		field  Field
		values []string
		valid  bool
	}{
		{"skip text", Field{Kind: Text}, nil, true},
		{"empty skip", Field{Kind: Text}, []string{}, true},
		{"text", Field{Kind: Text}, []string{" context, including spaces "}, true},
		{"empty text", Field{Kind: Text}, []string{""}, false},
		{"blank text", Field{Kind: Text}, []string{" \u00a0"}, false},
		{"multiple text", Field{Kind: Text}, []string{"a", "b"}, false},
		{"skip choice", Field{Kind: Choice, Options: []string{"a", "b"}}, nil, true},
		{"offered", Field{Kind: Choice, Options: []string{"a", "b"}}, []string{"a"}, true},
		{"unknown", Field{Kind: Choice, Options: []string{"a", "b"}}, []string{"c"}, false},
		{"single multiple", Field{Kind: Choice, Options: []string{"a", "b"}}, []string{"a", "b"}, false},
		{"multiple offered", Field{Kind: Choice, Options: []string{"a", "b"}, Multiple: true}, []string{"a", "b"}, true},
		{"custom comma", Field{Kind: Choice, Options: []string{"a", "b"}, AllowCustom: true}, []string{"custom, one"}, true},
		{"mixed custom", Field{Kind: Choice, Options: []string{"a", "b"}, Multiple: true, AllowCustom: true}, []string{"a", "custom, one"}, true},
		{"two custom", Field{Kind: Choice, Options: []string{"a", "b"}, Multiple: true, AllowCustom: true}, []string{"c", "d"}, false},
		{"duplicate offered", Field{Kind: Choice, Options: []string{"a", "b"}, Multiple: true}, []string{"a", "a"}, false},
		{"duplicate custom", Field{Kind: Choice, Options: []string{"a", "b"}, Multiple: true, AllowCustom: true}, []string{"c", "c"}, false},
		{"blank choice", Field{Kind: Choice, Options: []string{"a", "b"}, AllowCustom: true}, []string{"\u00a0"}, false},
		{"leading space", Field{Kind: Choice, Options: []string{"a", "b"}, AllowCustom: true}, []string{" a"}, false},
		{"trailing space", Field{Kind: Choice, Options: []string{"a", "b"}, AllowCustom: true}, []string{"c "}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			contract, err := New([]Field{test.field})
			if err != nil {
				t.Fatal(err)
			}
			before := slices.Clone(test.values)
			err = contract.Validate([][]string{test.values})
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%q) = %v, want valid=%t", test.values, err, test.valid)
			}
			if !slices.Equal(before, test.values) {
				t.Fatal("validation changed the submitted answer")
			}
		})
	}
}

func TestContractPreservesFieldIdentityAndRejectsIncompleteCandidates(t *testing.T) {
	contract, err := New([]Field{{Kind: Text}, {Kind: Choice, Options: []string{"a", "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range [][][]string{nil, {}, {{"one"}}, {{}, {}, {}}} {
		if err := contract.Validate(values); err == nil {
			t.Fatalf("invalid candidate accepted: %v", values)
		}
	}
	err = contract.Validate([][]string{{"one"}, {"other"}})
	field, ok := errors.AsType[*FieldError](err)
	if !ok || field.Index != 1 {
		t.Fatalf("field error = %v, want field index1", err)
	}
	if err := contract.Validate([][]string{{}, {}}); err != nil {
		t.Fatalf("explicit complete skip rejected: %v", err)
	}
}

func TestContractRejectsMalformedDefinitions(t *testing.T) {
	for _, fields := range [][]Field{nil, make([]Field, MaximumFields+1), {{}}, {{Kind: Text, AllowCustom: true}}, {{Kind: Choice, Options: []string{"a"}}}, {{Kind: Choice, Options: []string{"a", "a"}}}, {{Kind: Choice, Options: []string{"a", " b"}}}} {
		if _, err := New(fields); err == nil {
			t.Fatalf("invalid definition accepted: %+v", fields)
		}
	}
	if err := (Contract{}).Validate(nil); err == nil {
		t.Fatal("zero contract accepted answers")
	}
}

func TestContractDetachesTheAdvertisedAnswerSpace(t *testing.T) {
	fields := []Field{{Kind: Choice, Options: []string{"a", "b"}, Multiple: true}}
	contract, err := New(fields)
	if err != nil {
		t.Fatal(err)
	}
	fields[0].Options[0] = "changed"
	fields[0].Multiple = false
	var work sync.WaitGroup
	for range 8 {
		work.Go(func() {
			for range 32 {
				if err := contract.Validate([][]string{{"a", "b"}}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	work.Wait()
}
