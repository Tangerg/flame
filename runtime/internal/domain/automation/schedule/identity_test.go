package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestScheduleIdentityPolicyIsExactAndBounded(t *testing.T) {
	valid := []string{"sch_1", "sch_ABC-123"}
	for _, identity := range valid {
		if err := ValidateID(identity); err != nil {
			t.Errorf("ValidateID(%q): %v", identity, err)
		}
	}
	invalid := []string{
		"", "sch_", "other_1", " sch_1", "sch_1 ", "sch_ 1", "sch_\u200b1",
		string([]byte{0xff}), strings.Repeat("界", MaximumIDCharacters+1),
	}
	for _, identity := range invalid {
		if err := ValidateID(identity); err == nil {
			t.Errorf("ValidateID(%q) succeeded", identity)
		}
	}
}

func TestOccurrenceIdentityOwnsScheduleAndDueCursor(t *testing.T) {
	scheduleID, err := parseScheduleID("sch_1")
	if err != nil {
		t.Fatal(err)
	}
	dueAt := time.UnixMilli(1_725_000_000_123).UTC()
	identity, err := newOccurrenceIdentity(scheduleID, dueAt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseOccurrenceIdentity(identity.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.scheduleID != scheduleID || parsed.dueMillis != dueAt.UnixMilli() {
		t.Fatalf("parsed identity = %+v", parsed)
	}

	invalid := []string{
		"", "sch_1", "sch_1:", "sch_1:0", "sch_1:-1", "sch_1:01",
		"sch_ 1:1725000000123", "sch_1:1725000000123\u200b",
		strings.Repeat("界", MaximumOccurrenceIDCharacters+1),
	}
	for _, value := range invalid {
		if _, err := parseOccurrenceIdentity(value); err == nil {
			t.Errorf("parseOccurrenceIdentity(%q) succeeded", value)
		}
	}
}
