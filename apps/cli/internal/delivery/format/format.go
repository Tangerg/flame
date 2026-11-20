// Package format renders the few values the terminal and command renderers
// both display, so a span, a grouped count, a dollar amount and a usage summary
// read the same way wherever they appear.
package format

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// Duration renders a span the way a person reads one: whole milliseconds below a
// second, then seconds to one decimal.
func Duration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// Thousands groups an integer with comma separators, preserving a leading sign.
func Thousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	first := 0
	if digits[0] == '-' {
		first = 1
	}
	for i := len(digits) - 3; i > first; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

// USD renders a dollar amount to four decimal places. Callers decide what an
// absent amount means, since that fallback differs by surface.
func USD(amount float64) string {
	return "$" + strconv.FormatFloat(amount, 'f', 4, 64)
}

// Usage summarizes a Run's consumption; an empty usage renders nothing.
func Usage(usage conversation.Usage) string {
	if usage.Empty() {
		return ""
	}
	parts := []string{"↑" + Thousands(usage.InputTokens), "↓" + Thousands(usage.OutputTokens)}
	if usage.CacheReadTokens > 0 {
		parts = append(parts, "cached "+Thousands(usage.CacheReadTokens))
	}
	if usage.CostUSD != nil {
		parts = append(parts, USD(*usage.CostUSD))
	}
	if usage.Steps > 0 {
		parts = append(parts, "steps "+strconv.Itoa(usage.Steps))
	}
	if usage.Duration > 0 {
		parts = append(parts, Duration(usage.Duration))
	}
	return strings.Join(parts, "  ")
}
