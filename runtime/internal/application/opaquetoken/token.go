// Package opaquetoken frames small application-owned continuation values as
// strict, URL-safe tokens. It owns only the framing mechanism: callers own
// payload versions, invariants, scope checks, and malformed-input semantics.
//
// Tokens are opaque by contract, not secret or tamper-proof. Consumers store
// and return them verbatim; authorities decode and validate their own payload.
package opaquetoken

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrTooLarge reports that an encoded token would exceed its authority's
// declared resource envelope.
var ErrTooLarge = errors.New("opaque token exceeds maximum encoded size")

// Encode frames value as unpadded URL-safe Base64 around its JSON encoding.
// maximumCharacters is mandatory: the mechanism refuses to mint a token whose
// encoded output would exceed its authority's public or internal envelope.
func Encode(value any, maximumCharacters int) (string, error) {
	if maximumCharacters <= 0 {
		return "", errors.New("opaque token maximum must be positive")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if base64.RawURLEncoding.EncodedLen(len(payload)) > maximumCharacters {
		return "", fmt.Errorf("%w: maximum is %d characters", ErrTooLarge, maximumCharacters)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// Decode strictly decodes a bounded token into target. Unknown fields,
// trailing JSON values, malformed Base64, and invalid JSON are rejected.
// Payload-specific validation remains the caller's responsibility.
func Decode(token string, maximumCharacters int, target any) error {
	if maximumCharacters <= 0 {
		return errors.New("opaque token maximum must be positive")
	}
	if len(token) > maximumCharacters {
		return fmt.Errorf("%w: maximum is %d characters", ErrTooLarge, maximumCharacters)
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("opaque token contains a trailing JSON value")
		}
		return err
	}
	return nil
}
