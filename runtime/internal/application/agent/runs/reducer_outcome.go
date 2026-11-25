package runs

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (r *reducer) segmentEnd(e SegmentEnded) ([]ProjectionEvent, error) {
	r.segmentDuration = e.Duration
	terminal, err := r.finishedRun(e.Ending, e.unresolvedEffects)
	if err != nil {
		return nil, err
	}
	out, err := r.retainIncompleteModelOutput(ModelObservation{})
	if err != nil {
		return nil, err
	}
	drained, err := r.drainTools()
	if err != nil {
		return nil, err
	}
	out = append(out, drained...)
	return append(out, terminal), nil
}

func (r *reducer) parkedRun() (run.Run, error) {
	updatedAt := r.now()
	current, err := r.progressedRun(updatedAt)
	if err != nil {
		return run.Run{}, fmt.Errorf("project Run: %w", err)
	}
	return current.Suspend(updatedAt)
}

func (r *reducer) progressedRun(updatedAt time.Time) (run.Run, error) {
	metrics, err := r.metrics()
	if err != nil {
		return run.Run{}, err
	}
	return r.cfg.Opened.AdvanceProgress(metrics, r.contextTokens, updatedAt)
}

func (r *reducer) metrics() (run.Metrics, error) {
	activeDuration := r.cfg.Opened.Metrics().ActiveDuration()
	if r.segmentDuration < 0 || (r.segmentDuration > 0 && activeDuration > time.Duration(math.MaxInt64)-r.segmentDuration) {
		return run.Metrics{}, errors.New("segment active duration is invalid or overflows")
	}
	return r.accrued.WithActiveDuration(activeDuration + r.segmentDuration)
}

func (r *reducer) finishedRun(ending run.Ending, effects []run.UnresolvedEffect) (SegmentFinished, error) {
	finishedAt := r.now()
	current, err := r.progressedRun(finishedAt)
	if err != nil {
		return SegmentFinished{}, err
	}
	terminal, err := current.Terminate(run.Termination{
		Ending: ending, UnresolvedEffects: effects,
		FinishedAt: finishedAt,
	})
	if err != nil {
		return SegmentFinished{}, err
	}
	return SegmentFinished{Run: terminal}, nil
}
