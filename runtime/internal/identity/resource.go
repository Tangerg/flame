package identity

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

// MaximumResourceCharacters keeps every ordinary durable resource key bounded while
// leaving substantial headroom over Flame's current 36-character UUID entropy
// and future collision-safe encodings. Replay Event identities have a separate
// cursor-sized envelope because they carry an opaque resume position.
const MaximumResourceCharacters = 256

// MaximumEventCharacters is the separate public envelope for an opaque replay
// identity carrying a complete bounded cursor.
const MaximumEventCharacters = len(EventPrefix) + MaximumCursorCharacters
