package pagination

import (
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framevalue "github.com/Tangerg/frame/value"
)

var ErrTokenTooLarge = errors.New("opaque token exceeds maximum encoded size")

func EncodeToken(value any, maximumCharacters int) (string, error) {
	if maximumCharacters <= 0 {
		return "", errors.New("opaque token maximum must be positive")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if base64.RawURLEncoding.EncodedLen(len(payload)) > maximumCharacters {
		return "", fmt.Errorf("%w: maximum is %d characters", ErrTokenTooLarge, maximumCharacters)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeToken(token string, maximumCharacters int, target any) error {
	if maximumCharacters <= 0 {
		return errors.New("opaque token maximum must be positive")
	}
	if len(token) > maximumCharacters {
		return fmt.Errorf("%w: maximum is %d characters", ErrTokenTooLarge, maximumCharacters)
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, target, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
		return err
	}
	return nil
}
