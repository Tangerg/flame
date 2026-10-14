package protocol

import (
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// IDPrefixEvent frames the opaque replay cursor. Business plugin identities
// belong to their feature contracts, not to the Core protocol.
const IDPrefixEvent = runtimeidentity.EventPrefix

// MaximumResourceIdentityCharacters is the public envelope shared by opaque
// Session, Run, Segment, and Item identities. Clients retain and compare these
// values exactly; they must not normalize or parse them.
const MaximumResourceIdentityCharacters = runtimeidentity.MaximumResourceCharacters

// MaximumRunEventIDCharacters is the public resource envelope for the opaque
// event identity carried in Run events, subscription acknowledgements, in-process
// options, and the HTTP Last-Event-Id header. It includes the evt_ framing.
const MaximumRunEventIDCharacters = runtimeidentity.MaximumEventCharacters

// MaximumIdempotencyKeyCharacters bounds a client-chosen idempotency key where it
// travels as a request body field (runs.getAcceptance). The transport metadata
// path enforces its own byte limit; this is the wire-shape guard for the key as a
// lookup subject.
const MaximumIdempotencyKeyCharacters = 255

// ValidateSessionID reports whether value is an exact opaque Session identity.
func ValidateSessionID(value string) error {
	return runtimeidentity.ValidateResource("session", value, MaximumResourceIdentityCharacters)
}

// ValidateRunID reports whether value is an exact opaque Run identity.
func ValidateRunID(value string) error {
	return runtimeidentity.ValidateResource("run", value, MaximumResourceIdentityCharacters)
}

// ValidateSegmentID reports whether value is an exact opaque Segment identity.
func ValidateSegmentID(value string) error {
	return runtimeidentity.ValidateResource("segment", value, MaximumResourceIdentityCharacters)
}

// ValidateItemID reports whether value is an exact opaque Item identity.
func ValidateItemID(value string) error {
	return runtimeidentity.ValidateResource("item", value, MaximumResourceIdentityCharacters)
}

// ValidateRunEventID reports whether value is an exact opaque Run event identity.
func ValidateRunEventID(value string) error {
	return runtimeidentity.ValidateResource("event", value, MaximumRunEventIDCharacters)
}
