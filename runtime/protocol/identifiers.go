package protocol

import (
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// The two resource ID prefixes a client may rely on. A Schedule identity is
// constrained to IDPrefixSchedule on the wire, and IDPrefixEvent frames the
// opaque replay cursor. Every other prefix is a server generation convention:
// publishing it would invite exactly the parsing the identities below forbid.
const (
	IDPrefixSchedule = runtimeidentity.SchedulePrefix
	IDPrefixEvent    = runtimeidentity.EventPrefix
)

// MaximumResourceIdentityCharacters is the public envelope shared by opaque
// Session, Run, Segment, and Item identities. Clients retain and compare these
// values exactly; they must not normalize or parse them.
const MaximumResourceIdentityCharacters = runtimeidentity.MaximumResourceCharacters

// MaximumRunEventIDCharacters is the public resource envelope for the opaque
// event identity carried in Run events, subscription acknowledgements, in-process
// options, and the HTTP Last-Event-Id header. It includes the evt_ framing.
const MaximumRunEventIDCharacters = runtimeidentity.MaximumEventCharacters

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
