package model

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
)

const embeddingSpaceVersion = "embedding-v2:"

// embeddingSpaceID fingerprints every non-secret client input that can select
// a different vector coordinate system. Length framing preserves exact field
// boundaries without admitting separator collisions. Credentials are excluded:
// rotating an API key does not by itself invalidate persisted vectors.
func embeddingSpaceID(providerID, model, baseURL string) string {
	hash := sha256.New()
	for _, field := range []string{providerID, model, baseURL} {
		_, _ = hash.Write(binary.AppendUvarint(nil, uint64(len(field))))
		_, _ = io.WriteString(hash, field)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return embeddingSpaceVersion + hex.EncodeToString(digest[:])
}
