package runs

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

func TestSegmentItemIdentitiesBoundMaximumForeignSegment(t *testing.T) {
	segmentID := identity.SegmentPrefix + strings.Repeat(
		"s", identity.MaximumResourceCharacters-
			len(identity.SegmentPrefix),
	)
	if err := identity.ValidateSegment(segmentID); err != nil {
		t.Fatalf("maximum Segment fixture: %v", err)
	}

	identities := newSegmentItemIdentities(segmentID)
	first, err := identities.Next()
	if err != nil {
		t.Fatalf("first Item identity: %v", err)
	}
	if err := identity.ValidateItem(first); err != nil {
		t.Fatalf("Item identity derived from maximum Segment = %q: %v", first, err)
	}
	if user := userMessageItemID(segmentID); user == first {
		t.Fatalf("user Item identity aliases sequenced identity %q", user)
	} else if err := identity.ValidateItem(user); err != nil {
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
	if err := identity.ValidateItem(last); err != nil {
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

func TestSegmentItemIdentitiesRestoreCommittedAllocations(t *testing.T) {
	original := newSegmentItemIdentities("seg_original")
	first, _ := original.Next()
	second, _ := original.Next()
	third, _ := original.Next()
	foreign := newSegmentItemIdentities("seg_foreign")
	other, _ := foreign.Next()
	restored := newSegmentItemIdentities("seg_original")
	if err := restored.Restore([]string{second, other, first, userMessageItemID("seg_original")}); err != nil {
		t.Fatal(err)
	}
	if next, err := restored.Next(); err != nil || next != third {
		t.Fatalf("restored next=%q,%v, original=%q", next, err, third)
	}
	if err := restored.Restore([]string{strings.TrimSuffix(first, "1") + "01"}); err == nil {
		t.Fatal("noncanonical allocation was accepted")
	}
}
