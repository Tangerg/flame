package identity

import (
	"crypto/sha256"
	"io"
	"strconv"

	framevalue "github.com/Tangerg/frame/value"
)

func DigestStrings(fields ...string) framevalue.Digest {
	h := sha256.New()
	for _, field := range fields {
		_, _ = io.WriteString(h, strconv.Itoa(len(field))+":"+field)
	}
	return framevalue.DigestOf([sha256.Size]byte(h.Sum(nil)))
}
