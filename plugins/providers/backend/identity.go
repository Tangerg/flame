package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
)

func embeddingSpaceID(providerID, model, baseURL string) string {
	hash := sha256.New()
	for _, field := range []string{providerID, model, baseURL} {
		_, _ = hash.Write(binary.AppendUvarint(nil, uint64(len(field))))
		_, _ = io.WriteString(hash, field)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return hex.EncodeToString(digest[:])
}
