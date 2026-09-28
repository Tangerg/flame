package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/feedback"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
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
	if err := evidence.Snapshot.Session.ValidateFor(sessionID); err != nil {
		return TrajectoryExport{}, fmt.Errorf("sessions: trajectory Session identity: %w", err)
	}
	if err := evidence.Validate(); err != nil {
		return TrajectoryExport{}, err
	}
	view, err := e.coordinator.view(evidence.Snapshot.Session, ActivityIdle)
	if err != nil {
		return TrajectoryExport{}, err
	}
	return TrajectoryExport{Session: view, Evidence: evidence, CollectedAt: collectedAt}, nil
}

func (e TrajectoryEvidence) Validate() error {
	if err := e.Snapshot.Validate(); err != nil {
		return err
	}
	runIDs := make(map[string]struct{}, len(e.Snapshot.Runs))
	for _, run := range e.Snapshot.Runs {
		runIDs[run.ID()] = struct{}{}
	}
	callIDs := make(map[string]struct{}, len(e.ModelInvocations))
	for _, recorded := range e.ModelInvocations {
		if _, found := runIDs[recorded.RunID]; !found {
			return fmt.Errorf("sessions: trajectory model invocation names unknown Run %q", recorded.RunID)
		}
		invocation := recorded.Invocation
		if err := invocation.Validate(); err != nil {
			return fmt.Errorf("sessions: trajectory model invocation: %w", err)
		}
		if invocation.State == runs.ModelInvocationStarted {
			return errors.New("sessions: terminal trajectory contains an unsettled model invocation")
		}
		if _, found := callIDs[invocation.CallID]; found {
			return fmt.Errorf("sessions: duplicate trajectory model invocation %q", invocation.CallID)
		}
		callIDs[invocation.CallID] = struct{}{}
	}
	for _, entry := range e.Feedback {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("sessions: trajectory feedback: %w", err)
		}
	}
	return e.validateToolAttempts(runIDs)
}

func (e TrajectoryEvidence) validateToolAttempts(runIDs map[string]struct{}) error {
	type attemptIdentity struct{ callID, segmentID string }
	seen := make(map[attemptIdentity]struct{}, len(e.ToolAttempts))
	items := make(map[string]transcript.Item, len(e.Snapshot.Items))
	for _, item := range e.Snapshot.Items {
		items[item.ID()] = item
	}
	for _, recorded := range e.ToolAttempts {
		if _, found := runIDs[recorded.RunID]; !found {
			return fmt.Errorf("sessions: trajectory Tool attempt names unknown Run %q", recorded.RunID)
		}
		attempt := recorded.Invocation
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("sessions: trajectory Tool attempt: %w", err)
		}
		item, found := items[attempt.ItemID]
		if !found || item.Kind() != transcript.ToolCall || item.RunID() != recorded.RunID {
			return fmt.Errorf("sessions: trajectory Tool attempt %q has no matching Tool Item", attempt.CallID)
		}
		key := attemptIdentity{attempt.CallID, attempt.SegmentID}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("sessions: duplicate trajectory Tool attempt %q in Segment %q", attempt.CallID, attempt.SegmentID)
		}
		seen[key] = struct{}{}
	}
	return nil
}
