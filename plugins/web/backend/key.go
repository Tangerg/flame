package main

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type apiKey struct{ value string }

func newAPIKey(configured *string) (apiKey, error) {
	if configured == nil {
		return apiKey{}, nil
	}
	value := *configured
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || len(value) > 4096 || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return apiKey{}, errors.New("web: configured API key must be bounded, nonblank and contain no surrounding whitespace or control characters")
	}
	return apiKey{value: value}, nil
}

func (k apiKey) configured() bool               { return k.value != "" }
func (k apiKey) Format(state fmt.State, _ rune) { _, _ = fmt.Fprint(state, "[redacted]") }
