package runidentity

import (
	"strings"
	"testing"
)

func TestExecutionIdentitiesAreExactAndDistinct(t *testing.T) {
	run, err := ParseRun("run_一/2")
	if err != nil || run.String() != "run_一/2" {
		t.Fatalf("run identity = %q, %v", run.String(), err)
	}
	segment, err := ParseSegment("seg_三:4")
	if err != nil || segment.String() != "seg_三:4" {
		t.Fatalf("segment identity = %q, %v", segment.String(), err)
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
		if _, err := ParseRun(value); err == nil {
			t.Errorf("ParseRun(%q) accepted an invalid identity", value)
		}
		if _, err := ParseSegment(value); err == nil {
			t.Errorf("ParseSegment(%q) accepted an invalid identity", value)
		}
		if _, err := ParseItem(value); err == nil {
			t.Errorf("ParseItem(%q) accepted an invalid identity", value)
		}
		if _, err := ParseEvent(value); err == nil {
			t.Errorf("ParseEvent(%q) accepted an invalid identity", value)
		}
	}
	ordinaryOversize := strings.Repeat("一", MaximumCharacters+1)
	for name, parse := range map[string]func(string) error{
		"run":     func(value string) error { _, err := ParseRun(value); return err },
		"segment": func(value string) error { _, err := ParseSegment(value); return err },
		"item":    func(value string) error { _, err := ParseItem(value); return err },
	} {
		if err := parse(ordinaryOversize); err == nil {
			t.Errorf("%s parser accepted an oversized identity", name)
		}
	}
	if _, err := ParseEvent(strings.Repeat("一", MaximumEventCharacters+1)); err == nil {
		t.Error("event parser accepted an oversized identity")
	}
	if err := (RunID{}).Validate(); err == nil {
		t.Fatal("zero Run identity is valid")
	}
	if err := (SegmentID{}).Validate(); err == nil {
		t.Fatal("zero Segment identity is valid")
	}
	if err := (ItemID{}).Validate(); err == nil {
		t.Fatal("zero Item identity is valid")
	}
	if err := (EventID{}).Validate(); err == nil {
		t.Fatal("zero Event identity is valid")
	}
}
