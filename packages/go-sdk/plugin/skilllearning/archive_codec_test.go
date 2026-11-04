package skilllearning

import (
	"testing"
	"time"

	"github.com/Tangerg/frame/value"
)

func TestArchiveContractPreservesCapturedInstantsAndOptionalAdvice(t *testing.T) {
	method, err := ArchiveMethod()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Contract(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 6, 7, 8, 987654321, time.UTC)
	request, err := value.EncodeJSON(ArchiveRequest{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	var decoded ArchiveRequest
	if err := value.DecodeValue(request.Bytes(), &decoded, method.Name()); err != nil || !decoded.Now.Equal(now) {
		t.Fatalf("captured archive instant changed: %+v, %v", decoded, err)
	}
	for _, advice := range []ArchiveAdvice{{}, {InactiveSince: new(now.Add(-time.Nanosecond))}} {
		encoded, err := value.EncodeJSON(advice)
		if err != nil {
			t.Fatal(err)
		}
		var actual ArchiveAdvice
		if err := value.DecodeValue(encoded.Bytes(), &actual, "archive advice"); err != nil {
			t.Fatal(err)
		}
		if advice.InactiveSince == nil {
			if actual.InactiveSince != nil {
				t.Fatal("absent archival advice became a cutoff")
			}
		} else if actual.InactiveSince == nil || !actual.InactiveSince.Equal(*advice.InactiveSince) {
			t.Fatal("the absolute cutoff changed")
		}
	}
	for _, invalid := range []string{`{"now":null}`, `{}`, `{"now":"not a time"}`} {
		if err := value.DecodeValue([]byte(invalid), &decoded, "archive request"); err == nil {
			t.Fatalf("invalid captured instant admitted: %s", invalid)
		}
	}
}
