// Tokens are opaque by contract, not secret or tamper-proof. Consumers store
// and return them verbatim; authorities decode and validate their own payload.
package pagination

import (
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
)

var ErrTokenTooLarge = errors.New("opaque token exceeds maximum encoded size")

// maximumCharacters is mandatory: the mechanism refuses to mint a token whose
// encoded output would exceed its authority's public or internal envelope.
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

// Framing cannot establish payload-specific invariants; the caller must validate them.
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
	if err := json.Unmarshal(payload, target, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	return nil
}
