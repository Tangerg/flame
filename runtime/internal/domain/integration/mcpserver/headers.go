package mcpserver

import (
	"errors"
	"fmt"
	"strings"
)

func validateHTTPConfiguration(authorization string, headers map[string]string) error {
	if authorization != "" {
		if strings.TrimSpace(authorization) == "" || strings.TrimSpace(authorization) != authorization ||
			!validHeaderValue(authorization) {
			return errors.New("authorization is invalid")
		}
	}
	seen := make(map[string]string, len(headers))
	for name, value := range headers {
		if !validHeaderName(name) {
			return fmt.Errorf("headers name %q is invalid", name)
		}
		canonical := strings.ToLower(name)
		if canonical == "authorization" {
			return errors.New("headers must not duplicate authorization")
		}
		if previous, duplicate := seen[canonical]; duplicate {
			return fmt.Errorf("headers names %q and %q differ only by case", previous, name)
		}
		if !validHeaderValue(value) {
			return fmt.Errorf("headers value for %q is invalid", name)
		}
		seen[canonical] = name
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for index := range len(name) {
		character := name[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func validHeaderValue(value string) bool {
	for index := range len(value) {
		character := value[index]
		if (character >= 0x20 && character != 0x7f) || character == '\t' {
			continue
		}
		return false
	}
	return true
}
