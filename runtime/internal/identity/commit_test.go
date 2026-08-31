package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestRunCommitIdentityIsCanonicalAndBounded(t *testing.T) {
	generated := NewCommit()
	if generated.IsZero() {
		t.Fatal("New returned the absent identity")
	}
	if err := generated.Validate(); err != nil {
		t.Fatalf("Validate generated identity: %v", err)
	}
	if _, err := ParseCommit(generated.String()); err != nil {
		t.Fatalf("Parse generated identity: %v", err)
	}

	boundary := CommitPrefix + strings.Repeat("x", MaximumResourceCharacters-len(CommitPrefix))
	parsed, err := ParseCommit(boundary)
	if err != nil || parsed.String() != boundary {
		t.Fatalf("boundary identity = %q, %v", parsed.String(), err)
	}

	invalid := []string{
		"",
		CommitPrefix,
		"event_commit_1",
		CommitPrefix + "contains space",
		CommitPrefix + "contains/slash",
		strings.Repeat("x", MaximumResourceCharacters+1),
	}
	for _, value := range invalid {
		if _, err := ParseCommit(value); !errors.Is(err, ErrInvalidCommit) {
			t.Errorf("ParseCommit(%q) error = %v, want ErrInvalidCommit", value, err)
		}
	}
	if err := (CommitID{}).Validate(); !errors.Is(err, ErrInvalidCommit) || !(CommitID{}).IsZero() {
		t.Fatalf("zero identity validation = %v IsZero=%t, want ErrInvalidCommit/true", err, (CommitID{}).IsZero())
	}
}
