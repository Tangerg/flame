package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
)

var (
	digestExpression = regexp.MustCompile(fmt.Sprintf(`^[0-9a-f]{%d}$`, 2*sha256.Size))

	ErrInvalidDigest = errors.New("fingerprint: digest must be 64 lowercase hexadecimal characters")
)

// DigestPattern projects the canonical spelling for public contracts.
func DigestPattern() string { return digestExpression.String() }

// Digest is one SHA-256 value in its canonical lowercase hexadecimal spelling.
// Release contents and source authority share this representation; parsing
// here is the only place the spelling is checked. The zero value is no digest.
type Digest struct{ text string }

func ParseDigest(text string) (Digest, error) {
	if !digestExpression.MatchString(text) {
		return Digest{}, ErrInvalidDigest
	}
	return Digest{text: text}, nil
}

func Sum(sum [sha256.Size]byte) Digest { return Digest{text: hex.EncodeToString(sum[:])} }

func (d Digest) String() string { return d.text }
func (d Digest) IsZero() bool   { return d.text == "" }

func (d Digest) Validate() error {
	if d.text == "" {
		return ErrInvalidDigest
	}
	return nil
}

// Empty text is the encoding of the zero value, so a stored "no digest"
// decodes to exactly what was encoded and every other spelling is parsed.
func (d Digest) MarshalText() ([]byte, error) { return []byte(d.text), nil }

func (d *Digest) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*d = Digest{}
		return nil
	}
	parsed, err := ParseDigest(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
