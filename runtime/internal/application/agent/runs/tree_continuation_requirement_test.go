package runs

import (
	"strings"
	"testing"
)

// TestResumedRoutesRequireTheirContinuation fixes where "does this Segment have
// a continuation at all" is answered. A fresh root has none and a resumed tree
// must have one, so the question belongs at the fork between them — not on every
// method of the value, which by then is always one newTreeContinuation returned.
func TestResumedRoutesRequireTheirContinuation(t *testing.T) {
	t.Parallel()

	err := validateResumedRouteRequest(segmentSpec{
		RunID: "run_1", SessionID: "ses_1", SegmentID: "segment_1",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "require a tree continuation") {
		t.Fatalf("resumed routes without a continuation = %v, want a refusal", err)
	}
}
