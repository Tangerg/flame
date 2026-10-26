package runs

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

func TestRunCommitIdentityIsCanonicalAndBounded(t *testing.T) {
	generated := newRunCommitID()
	if generated.IsZero() {
		t.Fatal("New returned the absent identity")
	}
	if err := generated.Validate(); err != nil {
		t.Fatalf("Validate generated identity: %v", err)
	}
	if _, err := ParseCommit(generated.String()); err != nil {
		t.Fatalf("Parse generated identity: %v", err)
	}

	boundary := commitPrefix + strings.Repeat("x", identity.MaximumResourceCharacters-len(commitPrefix))
	parsed, err := ParseCommit(boundary)
	if err != nil || parsed.String() != boundary {
		t.Fatalf("boundary identity = %q, %v", parsed.String(), err)
	}

	invalid := []string{
		"",
		commitPrefix,
		"event_commit_1",
		commitPrefix + "contains space",
		commitPrefix + "contains/slash",
		strings.Repeat("x", identity.MaximumResourceCharacters+1),
	}
	for _, value := range invalid {
		if _, err := ParseCommit(value); !errors.Is(err, errInvalidCommit) {
			t.Errorf("ParseCommit(%q) error = %v, want errInvalidCommit", value, err)
		}
	}
	if err := (CommitID{}).Validate(); !errors.Is(err, errInvalidCommit) || !(CommitID{}).IsZero() {
		t.Fatalf("zero identity validation = %v IsZero=%t, want errInvalidCommit/true", err, (CommitID{}).IsZero())
	}
}
