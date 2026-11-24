package identity

// Consumers treat these identities as opaque and must not infer behavior from the
// prefixes.
const (
	sessionPrefix = "ses_"
	runPrefix     = "run_"
	segmentPrefix = "seg_"
	projectPrefix = "prj_"
	ItemPrefix    = "item_"
	EventPrefix   = "evt_"
)

func NewSession(entropy string) (SessionID, error) { return ParseSession(sessionPrefix + entropy) }
func NewRun(entropy string) (RunID, error)         { return ParseRun(runPrefix + entropy) }
func NewSegment(entropy string) (SegmentID, error) { return ParseSegment(segmentPrefix + entropy) }
func NewItem(entropy string) (ItemID, error)       { return ParseItem(ItemPrefix + entropy) }
func NewProject(entropy string) (ProjectID, error) { return ParseProject(projectPrefix + entropy) }

const MaximumResourceCharacters = 256

const MaximumEventCharacters = len(EventPrefix) + MaximumCursorCharacters
