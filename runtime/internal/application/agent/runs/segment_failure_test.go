package runs

import "github.com/Tangerg/flame/runtime/internal/domain/run"

// segmentFailure returns the classified failure a Segment ended with, if any.
func segmentFailure(end SegmentEnded) *run.Failure {
	failed, ok := end.Ending.(run.Failed)
	if !ok {
		return nil
	}
	return &failed.Failure
}
