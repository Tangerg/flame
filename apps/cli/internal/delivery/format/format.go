package format

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// Separator joins inline metadata wherever the CLI presents it.
const Separator = " · "

func Duration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

const (
	thousandsGroupDigits = 3
	day                  = 24 * time.Hour
)

// Under a minute a duration is recent and carries no amount; otherwise it is
// counted in its largest whole unit.
func Elapsed(d time.Duration) (amount int, unit string, recent bool) {
	switch {
	case d < time.Minute:
		return 0, "", true
	case d < time.Hour:
		return int(d / time.Minute), "m", false
	case d < day:
		return int(d / time.Hour), "h", false
	default:
		return int(d / day), "d", false
	}
}

func Thousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	first := 0
	if digits[0] == '-' {
		first = 1
	}
	for i := len(digits) - thousandsGroupDigits; i > first; i -= thousandsGroupDigits {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func USD(amount float64) string {
	return "$" + strconv.FormatFloat(amount, 'f', 4, 64)
}

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
