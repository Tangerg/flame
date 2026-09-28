package delivery

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
)

const SessionsExportTrajectory Name = "sessions.exportTrajectory"

type trajectoryExportUseCases interface {
	Export(context.Context, string) (sessions.TrajectoryExport, error)
}

func registerExportTrajectory(registry *Registry) {
	registry.query(MethodMeta{
		Name: SessionsExportTrajectory,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(), protocol.ErrSessionBusy.Error(),
			protocol.ErrExportTooLarge.Error(),
		},
		CapabilityRules: requires(protocol.FeatureSessionExport),
		Materializes:    []Name{SessionsGet, RunsList, ItemsList, ModelInvocationsList, PlanGet},
	}, func(service interface {
		ExportTrajectory(context.Context, protocol.ExportTrajectoryRequest) (*protocol.ExportTrajectoryResponse, error)
	}, ctx context.Context, request protocol.ExportTrajectoryRequest) (*protocol.ExportTrajectoryResponse, error) {
		return service.ExportTrajectory(ctx, request)
	})
}

func (s *Handler) ExportTrajectory(ctx context.Context, request protocol.ExportTrajectoryRequest) (*protocol.ExportTrajectoryResponse, error) {
	result, err := s.trajectoryExports.Export(ctx, request.SessionID)
	if err != nil {
		return nil, wireTrajectoryExportError(err)
	}
	for _, run := range result.Evidence.Snapshot.Runs {
		if run.Lineage().IsChild() {
			if err := s.requireFeature(ctx, protocol.FeatureSubagents); err != nil {
				return nil, err
			}
			break
		}
	}
	trajectory, err := presentTrajectoryExport(result)
	if err != nil {
		return nil, fmt.Errorf("sessions.exportTrajectory: encode evidence: %w", err)
	}
	response := &protocol.ExportTrajectoryResponse{Trajectory: trajectory}
	if err := json.MarshalWrite(&trajectorySizeWriter{remaining: sessions.MaximumTrajectoryExportBytes}, response); err != nil {
		return nil, wireTrajectoryExportError(err)
	}
	return response, nil
}

func wireTrajectoryExportError(err error) error {
	if errors.Is(err, sessions.ErrSessionBusy) {
		return NewFailure(errors.Join(protocol.ErrSessionBusy, err), "finish or cancel the Session's active or waiting Run before exporting its trajectory")
	}
	if errors.Is(err, sessions.ErrExportTooLarge) {
		return NewFailure(errors.Join(protocol.ErrExportTooLarge, err), "trajectory export exceeds 64 MiB or 100000 stored records; no partial trajectory was exported")
	}
	return wireSessionErr(err)
}

func presentTrajectoryExport(result sessions.TrajectoryExport) (protocol.SessionTrajectory, error) {
	snapshot := result.Evidence.Snapshot
	out := protocol.SessionTrajectory{
		SchemaVersion: protocol.SessionTrajectoryVersion, CollectedAt: result.CollectedAt,
		Session: presentSession(result.Session),
		Runs:    make([]protocol.RunRef, 0, len(snapshot.Runs)), Items: make([]protocol.Item, 0, len(snapshot.Items)),
		Messages:         make([]jsontext.Value, 0, len(snapshot.Messages)),
		ToolResults:      make([]protocol.ArtifactToolResult, 0, len(snapshot.ToolResults)),
		Plan:             presentPlanStepList(snapshot.Plan),
		ModelInvocations: make([]protocol.ModelInvocation, 0, len(result.Evidence.ModelInvocations)),
		ToolAttempts:     make([]protocol.ToolAttempt, 0, len(result.Evidence.ToolAttempts)),
		Feedback:         make([]protocol.FeedbackEntry, 0, len(result.Evidence.Feedback)),
		Limitations: []string{
			"This document contains all retained records from one idle Session snapshot, including child Runs; it is not a transition log or an execution checkpoint.",
			"Model and Tool attempts deleted or never recorded by earlier versions cannot be reconstructed; missing records and usage do not mean zero calls or zero consumption.",
			"Messages are the current retained conversation after compaction; per-call prompts, provider request bodies, and auxiliary model spans are not recorded here.",
			"Feedback contains user-supplied signals whose references match this Session or its retained Runs and Items; references are unverified and may outlive their targets.",
			"Run completion and model-call completion describe execution outcomes, not answer quality or successful task evaluation.",
			"Run metrics include descendant accounting; adding root and child totals or combining Run totals with per-call usage double-counts consumption.",
			"Tool duration describes its final execution segment and excludes approval waits. Model timestamps measure Runtime observation boundaries; unknown settlements do not measure provider duration.",
			"Tool attempts retain each Segment separately: completed means a definite result was observed, incomplete may mean suspension for input, and started has no recorded settlement. These states do not rate result quality.",
		},
	}
	for _, run := range snapshot.Runs {
		out.Runs = append(out.Runs, presentRun(run))
	}
	for _, item := range snapshot.Items {
		out.Items = append(out.Items, presentItem(item))
	}
	for _, message := range snapshot.Messages {
		encoded, err := json.Marshal(message, json.Deterministic(true))
		if err != nil {
			return protocol.SessionTrajectory{}, err
		}
		out.Messages = append(out.Messages, encoded)
	}
	for _, blob := range snapshot.ToolResults {
		out.ToolResults = append(out.ToolResults, protocol.ArtifactToolResult{
			ID: blob.ID.String(), ItemID: blob.ItemID, ToolName: blob.ToolName,
			Preview: blob.Preview, Body: blob.Body, CreatedAt: blob.CreatedAt,
		})
	}
	for _, row := range result.Evidence.ModelInvocations {
		out.ModelInvocations = append(out.ModelInvocations, presentModelInvocation(row.RunID, row.Invocation))
	}
	for _, row := range result.Evidence.ToolAttempts {
		attempt := row.Invocation
		out.ToolAttempts = append(out.ToolAttempts, protocol.ToolAttempt{
			CallID: attempt.CallID, ItemID: attempt.ItemID, RunID: row.RunID, SegmentID: attempt.SegmentID,
			State: protocol.ToolAttemptState(attempt.State), StartedAt: attempt.StartedAt, SettledAt: attempt.FinishedAt,
		})
	}
	for _, entry := range result.Evidence.Feedback {
		out.Feedback = append(out.Feedback, protocol.FeedbackEntry{
			SessionID: entry.SessionID, RunID: entry.RunID, ItemID: entry.ItemID,
			Rating: protocol.FeedbackRating(entry.Rating), Text: entry.Text, CreatedAt: entry.CreatedAt,
		})
	}
	return out, nil
}

type trajectorySizeWriter struct{ remaining int }

func (w *trajectorySizeWriter) Write(value []byte) (int, error) {
	if len(value) > w.remaining {
		return 0, sessions.ErrExportTooLarge
	}
	w.remaining -= len(value)
	return len(value), nil
}
