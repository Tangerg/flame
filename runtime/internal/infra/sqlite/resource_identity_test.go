package sqlite

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

func TestResourceKeyAdmissionIsExactAndBounded(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		" padded",
		"interior space",
		"line\nbreak",
		"zero\u200bwidth",
		strings.Repeat("界", resourceidentity.MaximumCharacters+1),
		string([]byte{0xff}),
	}
	for _, value := range invalid {
		if err := validateSessionResource("test", value); err == nil {
			t.Errorf("Session key %q was accepted", value)
		}
		if err := validateRunResource("test", value); err == nil {
			t.Errorf("Run key %q was accepted", value)
		}
		if err := validateSegmentResource("test", value); err == nil {
			t.Errorf("Segment key %q was accepted", value)
		}
		if err := validateItemResource("test", value); err == nil {
			t.Errorf("Item key %q was accepted", value)
		}
	}
	if err := validateOptionalSessionResource("test", ""); err != nil {
		t.Fatalf("absent optional Session key: %v", err)
	}
	if err := validateOptionalRunResource("test", ""); err != nil {
		t.Fatalf("absent optional Run key: %v", err)
	}
	for _, valid := range []struct {
		name  string
		value string
		check func(string, string) error
	}{
		{name: "Session", value: "ses_一/2", check: validateSessionResource},
		{name: "Run", value: "run_一/2", check: validateRunResource},
		{name: "Segment", value: "seg_一/2", check: validateSegmentResource},
		{name: "Item", value: "item_一/2", check: validateItemResource},
	} {
		if err := valid.check("test", valid.value); err != nil {
			t.Errorf("valid %s key: %v", valid.name, err)
		}
	}
}
