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

const (
	segmentItemSeparator         = "_"
	userMessageItemDiscriminator = "user"
)

var errItemIdentitySequenceExhausted = errors.New("runs: segment Item identity sequence exhausted")

func derivedItemID(entropy string) identity.ItemID {
	id, err := identity.NewItem(entropy)
	if err != nil {
		panic(fmt.Sprintf("runs: derived Item identity %q is invalid: %v", entropy, err))
	}
	return id
}

func SteerItemID(commandKey string) identity.ItemID {
	digest := sha256.Sum256([]byte("steer:" + commandKey))
	return derivedItemID(hex.EncodeToString(digest[:]))
}

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
	return derivedItemID(i.segmentDigest + segmentItemSeparator + strconv.FormatUint(i.sequence, 10)), nil
}

func (i *segmentItemIdentities) Restore(ids []identity.ItemID) error {
	prefix := identity.ItemPrefix + i.segmentDigest + segmentItemSeparator
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
	return derivedItemID(identities.segmentDigest + segmentItemSeparator + userMessageItemDiscriminator)
}
