package execution

import (
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// segmentFailure returns the classified failure a Segment ended with, if any.
func segmentFailure(end runs.SegmentEnded) *run.Failure {
	failed, ok := end.Ending.(run.Failed)
	if !ok {
		return nil
	}
	return &failed.Failure
}
