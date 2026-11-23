package goalref

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const MaximumIncarnationCharacters = 128

type IncarnationID struct {
	text string
}

func ParseIncarnation(text string) (IncarnationID, error) {
	if err := identity.ValidateResource("goal incarnation", text, MaximumIncarnationCharacters); err != nil {
		return IncarnationID{}, err
	}
	return IncarnationID{text: text}, nil
}

func ParseOptionalIncarnation(text string) (IncarnationID, bool, error) {
	if text == "" {
		return IncarnationID{}, false, nil
	}
	parsed, err := ParseIncarnation(text)
	return parsed, err == nil, err
}

func (i IncarnationID) String() string { return i.text }

func (i IncarnationID) IsZero() bool { return i.text == "" }

func (i IncarnationID) Validate() error {
	if i.text == "" {
		return errors.New("goal incarnation identity is empty")
	}
	return nil
}
