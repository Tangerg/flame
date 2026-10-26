package dependency

import "testing"

type collaborator interface{ Do() }

type concrete struct{}

func (*concrete) Do() {}

// TestMissingSeesThroughAnInterfaceHoldingANilPointer covers the case this
// package exists for. The other rows are the ones a hand-written check tends to
// drop: a nil map, slice, channel or func is as absent as a nil pointer, and a
// present zero value is not absent at all.
func TestMissingSeesThroughAnInterfaceHoldingANilPointer(t *testing.T) {
	t.Parallel()

	var nilPointer *concrete
	var present collaborator = &concrete{}
	var typedNil collaborator = nilPointer

	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "untyped nil", value: nil, want: true},
		{name: "interface holding a nil pointer", value: typedNil, want: true},
		{name: "nil map", value: map[string]int(nil), want: true},
		{name: "nil slice", value: []int(nil), want: true},
		{name: "nil channel", value: (chan int)(nil), want: true},
		{name: "nil func", value: (func())(nil), want: true},
		{name: "present implementation", value: present, want: false},
		{name: "empty map", value: map[string]int{}, want: false},
		{name: "zero struct", value: concrete{}, want: false},
		{name: "zero int", value: 0, want: false},
		{name: "empty string", value: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Missing(test.value); got != test.want {
				t.Fatalf("Missing(%#v) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}
