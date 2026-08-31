package identity

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func TestBuildIdentityIsExactCanonicalSHA256(t *testing.T) {
	digest := sha256.Sum256([]byte("flame"))
	want := BuildFromSHA256(digest).String()
	parsed, err := ParseBuild(want)
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
		if _, err := ParseBuild(raw); !errors.Is(err, ErrInvalidBuild) {
			t.Errorf("ParseBuild(%q) error = %v, want ErrInvalidBuild", raw, err)
		}
	}
}

const hexDigestCharacters = sha256.Size * 2
