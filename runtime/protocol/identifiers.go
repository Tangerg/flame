package protocol

import (
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// Server-generated resource ID prefixes.
const (
	IDPrefixSession  = runtimeidentity.SessionPrefix
	IDPrefixRun      = runtimeidentity.RunPrefix
	IDPrefixSegment  = runtimeidentity.SegmentPrefix
	IDPrefixItem     = runtimeidentity.ItemPrefix
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
	_, err := resourceid.ParseSession(value)
	return err
}

// ValidateRunID reports whether value is an exact opaque Run identity.
func ValidateRunID(value string) error {
	_, err := resourceid.ParseRun(value)
	return err
}

// ValidateSegmentID reports whether value is an exact opaque Segment identity.
func ValidateSegmentID(value string) error {
	_, err := resourceid.ParseSegment(value)
	return err
}

// ValidateItemID reports whether value is an exact opaque Item identity.
func ValidateItemID(value string) error {
	_, err := resourceid.ParseItem(value)
	return err
}

// ValidateRunEventID reports whether value is an exact opaque Run event identity.
func ValidateRunEventID(value string) error {
	_, err := resourceid.ParseEvent(value)
	return err
}
