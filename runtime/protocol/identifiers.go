package protocol

import runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"

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
