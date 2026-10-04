package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// Strings preserves field boundaries, including empty fields and UTF-8 bytes.
// Callers own field selection and order; this package owns only their encoding.
func Strings(fields ...string) string {
	h := sha256.New()
	for _, field := range fields {
		_, _ = io.WriteString(h, strconv.Itoa(len(field))+":"+field)
	}
	return hex.EncodeToString(h.Sum(nil))
}
