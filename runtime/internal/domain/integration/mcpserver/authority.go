package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// AuthorityFingerprint excludes credentials and connection lifecycle settings:
// a token rotation does not grant authority to a different endpoint.
func (s Server) AuthorityFingerprint() string {
	fields := []string{string(s.Transport)}
	if s.Transport == TransportStreamableHTTP {
		fields = append(fields, s.URL)
	} else {
		fields = append(fields, s.Command, s.Dir)
		fields = append(fields, s.Args...)
	}
	h := sha256.New()
	for _, field := range fields {
		h.Write([]byte(strconv.Itoa(len(field)) + ":" + field))
	}
	return hex.EncodeToString(h.Sum(nil))
}
