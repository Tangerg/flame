package identity

import (
	"crypto/sha256"
	"io"
	"strconv"

	"github.com/Tangerg/qimu/integrity"
)

// Length framing preserves field boundaries, including empty fields and UTF-8 bytes.
// Callers own field selection and order; this package owns only their encoding.
func DigestStrings(fields ...string) integrity.Digest {
	h := sha256.New()
	for _, field := range fields {
		_, _ = io.WriteString(h, strconv.Itoa(len(field))+":"+field)
	}
	return integrity.DigestOf([sha256.Size]byte(h.Sum(nil)))
}
