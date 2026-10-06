package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/feedback"
)

const (
	MaximumTrajectoryExportBytes   = 64 << 20
	MaximumTrajectoryExportRecords = 100_000
)

var ErrExportTooLarge = errors.New("sessions: trajectory export exceeds resource limits")

type RecordedModelInvocation struct {
	RunID      string
	Invocation runs.ModelInvocationCommit
}

type RecordedToolAttempt struct {
	RunID      string
	Invocation runs.ToolInvocationCommit
}

type TrajectoryEvidence struct {
	Snapshot         Snapshot
	ModelInvocations []RecordedModelInvocation
	ToolAttempts     []RecordedToolAttempt
	Feedback         []feedback.Entry
}

// ReadTrajectoryExport must read every collection in one transaction and reject
// resource limits before materializing unbounded stored bodies.
type TrajectoryExportReader interface {
	ReadTrajectoryExport(context.Context, string) (TrajectoryEvidence, error)
}

type TrajectoryExport struct {
	Session     View
	Evidence    TrajectoryEvidence
	CollectedAt time.Time
}

type TrajectoryExporter struct {
	coordinator *Coordinator
	reader      TrajectoryExportReader
}

func NewTrajectoryExporter(coordinator *Coordinator, reader TrajectoryExportReader) (*TrajectoryExporter, error) {
	if coordinator == nil || dependency.Missing(reader) {
		return nil, errors.New("sessions: trajectory export requires session admission and snapshot reader")
	}
	return &TrajectoryExporter{coordinator: coordinator, reader: reader}, nil
}

func (e *TrajectoryExporter) Export(ctx context.Context, sessionID string) (TrajectoryExport, error) {
	admission, err := e.coordinator.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return TrajectoryExport{}, err
	}
	defer admission.Release()

	evidence, err := e.reader.ReadTrajectoryExport(ctx, sessionID)
	if err != nil {
		return TrajectoryExport{}, err
	}
	collectedAt := time.Now().UTC()
	view, err := e.coordinator.view(evidence.Snapshot.Session, ActivityIdle)
	if err != nil {
		return TrajectoryExport{}, err
	}
	return TrajectoryExport{Session: view, Evidence: evidence, CollectedAt: collectedAt}, nil
}
