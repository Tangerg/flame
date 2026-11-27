package protocol

import (
	"errors"
	"fmt"
	"regexp"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
)

const IDPrefixEvent = flameidentity.EventPrefix

const MaximumResourceIdentityCharacters = flameidentity.MaximumResourceCharacters

const MaximumRunEventIDCharacters = flameidentity.MaximumEventCharacters

const MaximumIdempotencyKeyCharacters = 255

// Keys must round-trip unchanged through both an HTTP field and JSON.
const IdempotencyKeyPattern = `^[!-~]+$`

var idempotencyKeyPattern = regexp.MustCompile(IdempotencyKeyPattern)

func ValidateIdempotencyKey(key string) error {
	switch {
	case len(key) > MaximumIdempotencyKeyCharacters:
		return fmt.Errorf("idempotency key must not exceed %d characters", MaximumIdempotencyKeyCharacters)
	case !idempotencyKeyPattern.MatchString(key):
		return errors.New("idempotency key must contain only visible ascii characters")
	default:
		return nil
	}
}

func ValidateSessionID(value string) error {
	return flameidentity.ValidateResource("session", value, MaximumResourceIdentityCharacters)
}

func ValidateRunID(value string) error {
	return flameidentity.ValidateResource("run", value, MaximumResourceIdentityCharacters)
}

func ValidateSegmentID(value string) error {
	return flameidentity.ValidateResource("segment", value, MaximumResourceIdentityCharacters)
}

func ValidateItemID(value string) error {
	return flameidentity.ValidateResource("item", value, MaximumResourceIdentityCharacters)
}

func ValidateRunEventID(value string) error {
	return flameidentity.ValidateResource("event", value, MaximumRunEventIDCharacters)
}
