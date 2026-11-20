// Package secret spells credential material at every redaction boundary, so
// diagnostics compare one vocabulary for presence and never reveal a value.
package secret

const (
	// Redacted stands in for credential material that is present.
	Redacted = "[REDACTED]"
	// Absent marks credential material that is not configured.
	Absent = "<absent>"
)

// Presence renders whether credential material exists without revealing it.
func Presence(present bool) string {
	if present {
		return Redacted
	}
	return Absent
}
