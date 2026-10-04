package plugin

import (
	"strings"
	"unicode/utf8"
)

func ValidResourcePath(name string) bool {
	if !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\x00<>\"|?*") || len(name) > 1024 {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		switch stem {
		case "CON", "PRN", "AUX", "NUL",
			"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM¹", "COM²", "COM³",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9", "LPT¹", "LPT²", "LPT³":
			return false
		}
		for _, char := range segment {
			if char < 32 || char == 127 {
				return false
			}
		}
	}
	return true
}
