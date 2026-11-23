package secrets

const (
	maskStars        = "****"
	maskEndsMinRunes = 9
)

// Either end is exposed only when at least five code points stay hidden, so a
// short multi-byte credential cannot reveal most of its value.
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
