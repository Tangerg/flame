package delivery

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

const runtimeInstancePrefix = "runtime_"

func NewInstanceID() string { return runtimeInstancePrefix + uuid.NewString() }

func ValidateInstanceID(text string) error {
	uuidText, ok := strings.CutPrefix(text, runtimeInstancePrefix)
	if !ok {
		return errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	parsed, err := uuid.Parse(uuidText)
	if err != nil || parsed.String() != uuidText {
		return errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	return nil
}
