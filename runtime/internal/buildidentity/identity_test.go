package buildidentity

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func TestBuildIdentityIsExactCanonicalSHA256(t *testing.T) {
	digest := sha256.Sum256([]byte("flame"))
	want := FromSHA256(digest).String()
	parsed, err := Parse(want)
	if err != nil {
		t.Fatalf("Parse generated identity: %v", err)
	}
	if parsed.String() != want {
		t.Fatalf("parsed identity = %q, want %q", parsed.String(), want)
	}

	invalid := []string{
		"",
		"sha256:build",
		"sha256:" + strings.Repeat("A", hexDigestCharacters),
		" sha256:" + strings.Repeat("0", hexDigestCharacters),
		"sha256:" + strings.Repeat("0", hexDigestCharacters) + " ",
		"sha256:" + strings.Repeat("0", hexDigestCharacters-1),
		"sha256:" + strings.Repeat("g", hexDigestCharacters),
	}
	for _, raw := range invalid {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalid", raw, err)
		}
	}
}

const hexDigestCharacters = sha256.Size * 2
