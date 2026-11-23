package protocol

import flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"

const IDPrefixEvent = flameidentity.EventPrefix

const MaximumResourceIdentityCharacters = flameidentity.MaximumResourceCharacters

const MaximumRunEventIDCharacters = flameidentity.MaximumEventCharacters

const MaximumIdempotencyKeyCharacters = 255

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
