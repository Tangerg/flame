package identity

import (
	"strings"
	"testing"
)

func TestExecutionIdentitiesAreExactAndDistinct(t *testing.T) {
	if err := ValidateRun("run_一/2"); err != nil {
		t.Fatalf("valid run identity: %v", err)
	}
	if err := ValidateSegment("seg_三:4"); err != nil {
		t.Fatalf("valid segment identity: %v", err)
	}
	for _, value := range []string{
		"",
		" run_1",
		"run_1 ",
		"run_ one",
		"run_\n",
		"run_\u200bhidden",
		"run_\x00one",
		string([]byte{0xff}),
	} {
		if err := ValidateRun(value); err == nil {
			t.Errorf("ValidateRun(%q) accepted an invalid identity", value)
		}
		if err := ValidateSegment(value); err == nil {
			t.Errorf("ValidateSegment(%q) accepted an invalid identity", value)
		}
		if err := ValidateItem(value); err == nil {
			t.Errorf("ValidateItem(%q) accepted an invalid identity", value)
		}
		if err := ValidateEvent(value); err == nil {
			t.Errorf("ValidateEvent(%q) accepted an invalid identity", value)
		}
	}
	ordinaryOversize := strings.Repeat("一", MaximumResourceCharacters+1)
	for name, parse := range map[string]func(string) error{
		"run":     ValidateRun,
		"segment": ValidateSegment,
		"item":    ValidateItem,
	} {
		if err := parse(ordinaryOversize); err == nil {
			t.Errorf("%s parser accepted an oversized identity", name)
		}
	}
	if err := ValidateEvent(strings.Repeat("一", MaximumEventCharacters+1)); err == nil {
		t.Error("event parser accepted an oversized identity")
	}
}
