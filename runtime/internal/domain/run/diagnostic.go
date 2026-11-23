package run

import (
	"strings"
	"unicode/utf8"
)

const (
	maximumDiagnosticBytes = 4000

	diagnosticEllipsis = "…"
)

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

func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	return boundDiagnostic(err.Error(), maximumDiagnosticBytes)
}
