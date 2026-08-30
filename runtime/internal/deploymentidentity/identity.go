// Package deploymentidentity owns the stable inputs hashed into Agent
// Framework deployment references.
package deploymentidentity

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const MaximumCharacters = 256

type value struct {
	text string
}

func parse(kind, text string) (value, error) {
	if text == "" {
		return value{}, fmt.Errorf("%s is empty", kind)
	}
	if !utf8.ValidString(text) {
		return value{}, fmt.Errorf("%s is not valid UTF-8", kind)
	}
	if characters := utf8.RuneCountInString(text); characters > MaximumCharacters {
		return value{}, fmt.Errorf("%s has %d characters, maximum is %d", kind, characters, MaximumCharacters)
	}
	for _, character := range text {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return value{}, fmt.Errorf("%s contains whitespace or a non-printing character", kind)
		}
	}
	return value{text: text}, nil
}

type Implementation struct{ value }

func ParseImplementation(text string) (Implementation, error) {
	parsed, err := parse("deployment implementation identity", text)
	return Implementation{value: parsed}, err
}

func (i Implementation) String() string { return i.text }

type Configuration struct{ value }

func ParseConfiguration(text string) (Configuration, error) {
	parsed, err := parse("deployment configuration identity", text)
	return Configuration{value: parsed}, err
}

func (i Configuration) String() string { return i.text }
