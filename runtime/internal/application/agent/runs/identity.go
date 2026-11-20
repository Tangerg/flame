package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// Resource identifiers are application-owned lifecycle identities. Their
// namespace is decided here rather than by composition or persistence.
const (
	runIDPrefix     = identity.RunPrefix
	segmentIDPrefix = identity.SegmentPrefix
	itemIDPrefix    = identity.ItemPrefix

	segmentItemSeparator         = "_"
	userMessageItemDiscriminator = "user"
)

var errItemIdentitySequenceExhausted = errors.New("runs: segment Item identity sequence exhausted")

// NewRunID, NewSegmentID, and NewItemID add the application-owned namespace to an opaque
// entropy value supplied by composition. The source may be UUID, a test
// sequence, or another collision-safe generator; the use case owns the
// resulting resource shape.
// NewRunID names a fresh Run from opaque entropy.
func NewRunID(entropy string) (identity.RunID, error) {
	return identity.ParseRun(runIDPrefix + entropy)
}

// NewSegmentID names a fresh Segment from opaque entropy.
func NewSegmentID(entropy string) (identity.SegmentID, error) {
	return identity.ParseSegment(segmentIDPrefix + entropy)
}

// NewItemID names a fresh Item from opaque entropy.
func NewItemID(entropy string) (identity.ItemID, error) {
	return identity.ParseItem(itemIDPrefix + entropy)
}

// derivedItemID admits an Item identity composed here from a digest, which
// always satisfies the Item grammar.
func derivedItemID(text string) identity.ItemID {
	id, err := identity.ParseItem(text)
	if err != nil {
		panic(fmt.Sprintf("runs: derived Item identity %q is invalid: %v", text, err))
	}
	return id
}

// SteerItemID nominates the same opaque Item for every delivery of one command.
// Nomination is read-only; only admission and a committed User Item establish
// acceptance and application respectively.
func SteerItemID(commandKey string) identity.ItemID {
	digest := sha256.Sum256([]byte("steer:" + commandKey))
	return derivedItemID(itemIDPrefix + hex.EncodeToString(digest[:]))
}

// segmentItemIdentities owns the deterministic Item namespace of one Segment.
// The Segment identity is digested before composition so every valid foreign
// Segment, including one at its maximum envelope, produces a bounded Item ID.
// The issuer is copied with a speculative reducer, preserving commit retry
// determinism without sharing mutable state.
type segmentItemIdentities struct {
	segmentDigest string
	sequence      uint64
}

func newSegmentItemIdentities(segmentID identity.SegmentID) segmentItemIdentities {
	digest := sha256.Sum256([]byte(segmentID.String()))
	return segmentItemIdentities{segmentDigest: hex.EncodeToString(digest[:])}
}

func (i *segmentItemIdentities) Next() (identity.ItemID, error) {
	if i.sequence == math.MaxUint64 {
		return identity.ItemID{}, errItemIdentitySequenceExhausted
	}
	i.sequence++
	return derivedItemID(itemIDPrefix + i.segmentDigest + segmentItemSeparator + strconv.FormatUint(i.sequence, 10)), nil
}

// The committed Items are the allocation ledger for an existing Segment.
// Restore observes that ledger; only Next may issue another identity.
func (i *segmentItemIdentities) Restore(ids []identity.ItemID) error {
	prefix := itemIDPrefix + i.segmentDigest + segmentItemSeparator
	var highest uint64
	for _, id := range ids {
		ordinal, owned := strings.CutPrefix(id.String(), prefix)
		if !owned || ordinal == userMessageItemDiscriminator {
			continue
		}
		sequence, err := strconv.ParseUint(ordinal, 10, 64)
		if err != nil || sequence == 0 || strconv.FormatUint(sequence, 10) != ordinal {
			return errors.New("runs: invalid committed Segment Item identity")
		}
		highest = max(highest, sequence)
	}
	i.sequence = highest
	return nil
}

func userMessageItemID(segmentID identity.SegmentID) identity.ItemID {
	identities := newSegmentItemIdentities(segmentID)
	return derivedItemID(itemIDPrefix + identities.segmentDigest + segmentItemSeparator + userMessageItemDiscriminator)
}
