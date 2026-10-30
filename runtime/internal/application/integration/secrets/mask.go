// Package secrets provides application-level handling for credential values
// that may cross a presentation or diagnostic boundary.
package secrets

const (
	maskStars        = "****"
	maskEndsMinRunes = 9
)

// Mask exposes either end only when at least five code points remain hidden,
// so a short multi-byte credential cannot reveal most of its value.
func Mask(value string) string {
	if value == "" {
		return ""
	}
	characters := []rune(value)
	if len(characters) < maskEndsMinRunes {
		return maskStars
	}
	return string(characters[:2]) + maskStars + string(characters[len(characters)-2:])
}
