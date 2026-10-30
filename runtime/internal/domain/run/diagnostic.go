package run

import (
	"strings"
	"unicode/utf8"
)

const (
	maximumDiagnosticBytes = 4000

	diagnosticEllipsis = "…"
)

// boundDiagnostic is the one rule for diagnostic text a person may read in a
// failure, loss or unresolved effect:
// valid UTF-8, bounded, and marked where it was cut, so a reader can tell a
// truncated diagnostic from a complete one. It returns the empty string for
// text that is only whitespace or only invalid bytes, which is what a caller
// substituting its own wording needs to know.
func boundDiagnostic(value string, maximumBytes int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if len(value) <= maximumBytes {
		return value
	}
	cut := maximumBytes - len(diagnosticEllipsis)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut]) + diagnosticEllipsis
}

// Diagnostic bounds an operational error for the failure detail a person sees.
func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	return boundDiagnostic(err.Error(), maximumDiagnosticBytes)
}
