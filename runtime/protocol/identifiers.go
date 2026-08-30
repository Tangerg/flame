package protocol

import "github.com/Tangerg/flame/runtime/internal/resourceidentity"

// Server-generated resource ID prefixes (doc/API.md §2.2).
const (
	IDPrefixSession  = resourceidentity.SessionPrefix
	IDPrefixRun      = resourceidentity.RunPrefix
	IDPrefixSegment  = resourceidentity.SegmentPrefix
	IDPrefixItem     = resourceidentity.ItemPrefix
	IDPrefixSchedule = resourceidentity.SchedulePrefix
	IDPrefixEvent    = resourceidentity.EventPrefix
)

// MaximumResourceIdentityCharacters is the public envelope shared by opaque
// Session, Run, Segment, and Item identities. Clients retain and compare these
// values exactly; they must not normalize or parse them.
const MaximumResourceIdentityCharacters = resourceidentity.MaximumCharacters

// MaximumRunEventIDCharacters is the public resource envelope for the opaque
// event identity carried in Run events, subscription acknowledgements, embedded
// options, and the HTTP Last-Event-Id header. It includes the evt_ framing.
const MaximumRunEventIDCharacters = resourceidentity.MaximumEventCharacters
