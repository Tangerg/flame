package runs

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestSegmentItemIdentitiesBoundMaximumForeignSegment(t *testing.T) {
	segmentID := runtimeidentity.SegmentPrefix + strings.Repeat(
		"s",
		runtimeidentity.MaximumResourceCharacters-len(runtimeidentity.SegmentPrefix),
	)
	if _, err := resourceid.ParseSegment(segmentID); err != nil {
		t.Fatalf("maximum Segment fixture: %v", err)
	}

	identities := newSegmentItemIdentities(segmentID)
	first, err := identities.Next()
	if err != nil {
		t.Fatalf("first Item identity: %v", err)
	}
	if _, err := resourceid.ParseItem(first); err != nil {
		t.Fatalf("Item identity derived from maximum Segment = %q: %v", first, err)
	}
	if user := userMessageItemID(segmentID); user == first {
		t.Fatalf("user Item identity aliases sequenced identity %q", user)
	} else if _, err := resourceid.ParseItem(user); err != nil {
		t.Fatalf("user Item identity derived from maximum Segment = %q: %v", user, err)
	}
}

func TestSegmentItemIdentitiesDoNotWrap(t *testing.T) {
	identities := newSegmentItemIdentities("seg_test")
	identities.sequence = math.MaxUint64 - 1
	last, err := identities.Next()
	if err != nil {
		t.Fatalf("last Item identity: %v", err)
	}
	if _, err := resourceid.ParseItem(last); err != nil {
		t.Fatalf("last Item identity = %q: %v", last, err)
	}
	if next, err := identities.Next(); next != "" || !errors.Is(err, errItemIdentitySequenceExhausted) {
		t.Fatalf("identity after exhaustion = %q, %v", next, err)
	}
}

func TestReducerPropagatesItemIdentityExhaustion(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	reducer.itemIDs.sequence = math.MaxUint64
	if _, err := reducer.reduce(MessageDelta{Text: "unpublishable"}); !errors.Is(err, errItemIdentitySequenceExhausted) {
		t.Fatalf("reducer error = %v, want Item identity exhaustion", err)
	}
}
