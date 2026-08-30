package commitidentity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

func TestRunCommitIdentityIsCanonicalAndBounded(t *testing.T) {
	generated := New()
	if generated.IsZero() {
		t.Fatal("New returned the absent identity")
	}
	if err := generated.Validate(); err != nil {
		t.Fatalf("Validate generated identity: %v", err)
	}
	if _, err := Parse(generated.String()); err != nil {
		t.Fatalf("Parse generated identity: %v", err)
	}

	boundary := Prefix + strings.Repeat("x", resourceidentity.MaximumCharacters-len(Prefix))
	parsed, err := Parse(boundary)
	if err != nil || parsed.String() != boundary {
		t.Fatalf("boundary identity = %q, %v", parsed.String(), err)
	}

	invalid := []string{
		"",
		Prefix,
		"event_commit_1",
		Prefix + "contains space",
		Prefix + "contains/slash",
		strings.Repeat("x", resourceidentity.MaximumCharacters+1),
	}
	for _, value := range invalid {
		if _, err := Parse(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalid", value, err)
		}
	}
	if err := (ID{}).Validate(); !errors.Is(err, ErrInvalid) || !(ID{}).IsZero() {
		t.Fatalf("zero identity validation = %v IsZero=%t, want ErrInvalid/true", err, (ID{}).IsZero())
	}
}
