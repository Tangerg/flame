package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/commitidentity"
	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

// Resource identifiers are application-owned lifecycle identities. Their
// namespace is decided here rather than by composition or persistence.
const (
	runIDPrefix     = resourceidentity.RunPrefix
	segmentIDPrefix = resourceidentity.SegmentPrefix
	itemIDPrefix    = resourceidentity.ItemPrefix

	segmentItemSeparator         = "_"
	userMessageItemDiscriminator = "user"
)

var errItemIdentitySequenceExhausted = errors.New("runs: segment Item identity sequence exhausted")

// NewRunID, NewSegmentID, and NewItemID add the application-owned namespace to an opaque
// entropy value supplied by composition. The source may be UUID, a test
// sequence, or another collision-safe generator; the use case owns the
// resulting resource shape.
func NewRunID(entropy string) string     { return runIDPrefix + entropy }
func NewSegmentID(entropy string) string { return segmentIDPrefix + entropy }
func NewItemID(entropy string) string    { return itemIDPrefix + entropy }

// segmentItemIdentities owns the deterministic Item namespace of one Segment.
// The Segment identity is digested before composition so every valid foreign
// Segment, including one at its maximum envelope, produces a bounded Item ID.
// The issuer is copied with a speculative reducer, preserving commit retry
// determinism without sharing mutable state.
type segmentItemIdentities struct {
	segmentDigest string
	sequence      uint64
}

func newSegmentItemIdentities(segmentID string) segmentItemIdentities {
	digest := sha256.Sum256([]byte(segmentID))
	return segmentItemIdentities{segmentDigest: hex.EncodeToString(digest[:])}
}

func (i *segmentItemIdentities) Next() (string, error) {
	if i.sequence == math.MaxUint64 {
		return "", errItemIdentitySequenceExhausted
	}
	i.sequence++
	return itemIDPrefix + i.segmentDigest + segmentItemSeparator + strconv.FormatUint(i.sequence, 10), nil
}

func userMessageItemID(segmentID string) string {
	identities := newSegmentItemIdentities(segmentID)
	return itemIDPrefix + identities.segmentDigest + segmentItemSeparator + userMessageItemDiscriminator
}

// newRunCommitID identifies one immutable top-level Run write-set. Terminal
// identities are minted where the reducer creates the write-set and retained
// across retries. Other command boundaries mint once before persistence.
func newRunCommitID() commitidentity.ID { return commitidentity.New() }
