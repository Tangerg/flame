// Package resourceidentity owns the transport-neutral envelope for Flame's
// opaque Session, Run, Segment, and Item identities. It depends on no
// architectural ring so Domain admission and public wire generation can share
// one resource policy.
package resourceidentity

import "github.com/Tangerg/flame/runtime/internal/cursorresource"

// Server-generated namespaces. Consumers treat the resulting identities as
// opaque and must not infer behavior from these prefixes.
const (
	SessionPrefix  = "ses_"
	RunPrefix      = "run_"
	SegmentPrefix  = "seg_"
	ItemPrefix     = "item_"
	SchedulePrefix = "sch_"
	EventPrefix    = "evt_"
)

// MaximumCharacters keeps every ordinary durable resource key bounded while
// leaving substantial headroom over Flame's current 36-character UUID entropy
// and future collision-safe encodings. Replay Event identities have a separate
// cursor-sized envelope because they carry an opaque resume position.
const MaximumCharacters = 256

// MaximumEventCharacters is the separate public envelope for an opaque replay
// identity carrying a complete bounded cursor.
const MaximumEventCharacters = len(EventPrefix) + cursorresource.MaximumCharacters
