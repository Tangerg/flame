package capture

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type APIKey struct{ value string }

func NewAPIKey(configured *string) (APIKey, error) {
	if configured == nil {
		return APIKey{}, nil
	}
	value := *configured
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || len(value) > 4096 || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return APIKey{}, errors.New("web: configured API key must be bounded, nonblank and contain no surrounding whitespace or control characters")
	}
	return APIKey{value: value}, nil
}

func (k APIKey) Configured() bool               { return k.value != "" }
func (k APIKey) Value() string                  { return k.value }
func (k APIKey) Format(state fmt.State, _ rune) { _, _ = fmt.Fprint(state, "[redacted]") }
