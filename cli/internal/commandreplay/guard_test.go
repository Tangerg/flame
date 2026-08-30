package commandreplay

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGuardRequiresAnExplicitProtectionKind(t *testing.T) {
	t.Parallel()

	if err := (Guard{}).Validate(); err == nil {
		t.Fatal("zero Guard was valid")
	}
	unprotected := UnprotectedGuard()
	if err := unprotected.Validate(); err != nil || unprotected.Protected() || unprotected.Kind() != GuardUnprotected {
		t.Fatalf("unprotected guard = %+v, err %v", unprotected, err)
	}
	deadline := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	protected, err := NewProtectedGuard(" runtime-a ", deadline)
	if err != nil {
		t.Fatal(err)
	}
	if !protected.Protected() || protected.Namespace() != "runtime-a" || !protected.Until().Equal(deadline) {
		t.Fatalf("protected guard = %+v", protected)
	}
}

func TestGuardJSONUsesOneStrictExplicitUnion(t *testing.T) {
	t.Parallel()

	deadline := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	protected, err := NewProtectedGuard("runtime-a", deadline)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []Guard{UnprotectedGuard(), protected} {
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got Guard
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("round trip = %+v, want %+v", got, want)
		}
	}
	for _, invalid := range []string{
		`{}`,
		`{"type":"unprotected","namespace":"runtime-a"}`,
		`{"type":"unprotected","until":"2026-08-29T12:00:00Z"}`,
		`{"type":"protected","namespace":"runtime-a"}`,
		`{"type":"protected","namespace":"","until":"2026-08-29T12:00:00Z"}`,
		`{"type":"protected","namespace":"runtime-a","until":"2026-08-29T12:00:00Z","extra":true}`,
		`{"type":"protected","namespace":"runtime-a","until":"2026-08-29T12:00:00Z"} {}`,
	} {
		var decoded Guard
		if err := json.Unmarshal([]byte(invalid), &decoded); err == nil {
			t.Fatalf("decoded invalid guard %s", invalid)
		}
	}
}
