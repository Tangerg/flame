package persistence

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/samber/lo"
)

type TrajectoryExportConfig struct {
	Snapshots        sessions.SnapshotReader
	Sessions         *SessionStore
	ModelInvocations *modelInvocationReader
	ToolInvocations  *ToolInvocationStore
	Feedback         *feedbackStore
	ReadSnapshot     Transactor
}

type trajectoryExportReader struct {
	snapshots        sessions.SnapshotReader
	sessions         *SessionStore
	modelInvocations *modelInvocationReader
	toolInvocations  *ToolInvocationStore
	feedback         *feedbackStore
	readSnapshot     Transactor
}

func NewTrajectoryExportReader(cfg TrajectoryExportConfig) (*trajectoryExportReader, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"snapshots", cfg.Snapshots}, {"sessions", cfg.Sessions},
		{"model invocations", cfg.ModelInvocations}, {"Tool invocations", cfg.ToolInvocations},
		{"feedback", cfg.Feedback}, {"read snapshot", cfg.ReadSnapshot},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("persistence: trajectory export %s is required", required.name)
		}
	}
	return &trajectoryExportReader{
		snapshots: cfg.Snapshots, sessions: cfg.Sessions, modelInvocations: cfg.ModelInvocations,
		toolInvocations: cfg.ToolInvocations, feedback: cfg.Feedback, readSnapshot: cfg.ReadSnapshot,
	}, nil
}

func (r *trajectoryExportReader) ReadTrajectoryExport(ctx context.Context, sessionID string) (sessions.TrajectoryEvidence, error) {
	var evidence sessions.TrajectoryEvidence
	err := r.readSnapshot(ctx, func(ctx context.Context) error {
		if _, err := r.sessions.Get(ctx, sessionID); err != nil {
			return err
		}
		records, bytes, err := r.sessions.TrajectoryExportSize(ctx, sessionID)
		if err != nil {
			return err
		}
		if records > sessions.MaximumTrajectoryExportRecords || bytes > sessions.MaximumTrajectoryExportBytes {
			return sessions.ErrExportTooLarge
		}
		evidence.Snapshot, err = r.snapshots.ReadSnapshot(ctx, sessionID)
		if err != nil {
			return err
		}
		if err := evidence.Snapshot.Validate(); err != nil {
			return err
		}
		for _, run := range evidence.Snapshot.Runs {
			if err := r.readModelInvocations(ctx, run.ID(), &evidence); err != nil {
				return err
			}
		}
		evidence.ModelEndpoints, err = r.modelInvocations.store.ListSessionEndpoints(ctx, sessionID)
		if err != nil {
			return err
		}
		if err := r.readToolAttempts(ctx, sessionID, &evidence); err != nil {
			return err
		}
		evidence.Feedback, err = r.feedback.ListSession(ctx, sessionID)
		return err
	})
	if err != nil {
		return sessions.TrajectoryEvidence{}, err
	}
	slices.SortFunc(evidence.ModelInvocations, func(a, b sessions.RecordedModelInvocation) int {
		if order := a.Invocation.StartedAt.Compare(b.Invocation.StartedAt); order != 0 {
			return order
		}
		return strings.Compare(a.Invocation.CallID, b.Invocation.CallID)
	})
	return evidence, nil
}

func (r *trajectoryExportReader) readToolAttempts(ctx context.Context, sessionID string, evidence *sessions.TrajectoryEvidence) error {
	rows, err := r.toolInvocations.ListSession(ctx, sessionID)
	if err != nil {
		return err
	}
	evidence.ToolAttempts = make([]sessions.RecordedToolAttempt, 0, len(rows))
	for _, row := range rows {
		attempt := runs.ToolInvocationCommit{
			CallID: row.CallID, ItemID: row.ItemID, SegmentID: row.SegmentID,
			State: runs.ToolInvocationState(row.State), StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		}
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("persistence: restore trajectory Tool attempt: %w", err)
		}
		evidence.ToolAttempts = append(evidence.ToolAttempts, sessions.RecordedToolAttempt{RunID: row.RunID, Invocation: attempt})
	}
	return nil
}

func (r *trajectoryExportReader) readModelInvocations(ctx context.Context, runID string, evidence *sessions.TrajectoryEvidence) error {
	const pageSize = 100
	var beforeStartedAt int64
	var beforeCallID string
	for {
		rows, err := r.modelInvocations.PageModelInvocations(ctx, runID, beforeStartedAt, beforeCallID, pageSize)
		if err != nil {
			return err
		}
		for _, row := range rows {
			evidence.ModelInvocations = append(evidence.ModelInvocations, sessions.RecordedModelInvocation{RunID: runID, Invocation: row})
		}
		if len(rows) < pageSize {
			return nil
		}
		last := rows[len(rows)-1]
		beforeStartedAt, beforeCallID = last.StartedAt.UnixNano(), last.CallID
	}
}
