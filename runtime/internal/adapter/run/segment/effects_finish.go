package segment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	titleMaintenance      = "title"
	checkpointMaintenance = "checkpoint"
)

type sessionTitles interface {
	PrepareGeneratedTitle(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) (*sessions.GeneratedTitle, error)
}

type checkpoints interface {
	Snapshot(ctx context.Context, sessionID identity.SessionID, cwd string, runID identity.RunID) error
}

type taskLauncher interface {
	Start(parent context.Context, task func(context.Context)) bool
}

type TitleMaintenance struct {
	Sessions sessionTitles
	Model    modeladapter.AuxiliaryResolver
	Tasks    taskLauncher
}

type FinalizerConfig struct {
	Checkpoints checkpoints
	Titles      TitleMaintenance
}

type Finalizer struct {
	checkpoints   checkpoints
	sessionTitles sessionTitles
	titles        titleGenerator
	tasks         taskLauncher
}

var _ runs.SegmentFinalizer = (*Finalizer)(nil)

func NewFinalizer(cfg FinalizerConfig) (*Finalizer, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"checkpoints", cfg.Checkpoints},
		{"session titles", cfg.Titles.Sessions},
		{"title model", cfg.Titles.Model},
		{"task launcher", cfg.Titles.Tasks},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("segment: %s are required", required.name)
		}
	}
	return &Finalizer{
		checkpoints:   cfg.Checkpoints,
		sessionTitles: cfg.Titles.Sessions,
		titles:        titleGenerator{model: cfg.Titles.Model},
		tasks:         cfg.Titles.Tasks,
	}, nil
}

// The checkpoint is a sequencing fence: admission stays held until it completes so
// a following run cannot write into this run's snapshot.
func (f *Finalizer) Finish(ctx context.Context, fin runs.Finish) error {
	needsSnapshot := !fin.Parked && fin.WorkspaceCWD != ""
	needsTitle := strings.TrimSpace(fin.OpeningUserText) != ""
	if !needsSnapshot && !needsTitle {
		return nil
	}
	ctx = executionctx.WithScope(ctx, runs.ExecutionScope{SessionID: fin.SessionID, CWD: fin.WorkspaceCWD, WorkspaceCWD: fin.WorkspaceCWD})
	ctx = executionctx.WithInvocationOwner(ctx, "")
	var errs []error
	if needsSnapshot {
		if err := observeSegmentMaintenance(ctx, fin, checkpointMaintenance, func(ctx context.Context) error {
			return f.snapshot(ctx, fin.SessionID, fin.WorkspaceCWD, fin.RunID)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	if !needsTitle {
		return errors.Join(errs...)
	}
	claim, err := f.sessionTitles.PrepareGeneratedTitle(ctx, fin.SessionID, fin.RunID)
	if err != nil {
		errs = append(errs, fmt.Errorf("segment: prepare generated title for Session %q: %w", fin.SessionID, err))
		return errors.Join(errs...)
	}
	if claim == nil {
		return errors.Join(errs...)
	}
	if !f.tasks.Start(ctx, func(ctx context.Context) {
		defer claim.Release()
		if err := observeSegmentMaintenance(ctx, fin, titleMaintenance, func(ctx context.Context) error {
			return f.title(ctx, claim, fin.SessionID, fin.OpeningUserText)
		}); err != nil {
			slog.WarnContext(ctx, "segment: maintain session title", telemetry.RunID(fin.RunID), telemetry.Error(err))
		}
	}) {
		claim.Release()
		rejected := fmt.Errorf("segment: maintenance for run %q was rejected during shutdown", fin.RunID)
		errs = append(errs, observeSegmentMaintenance(ctx, fin, titleMaintenance, func(context.Context) error { return rejected }))
	}
	return errors.Join(errs...)
}

func observeSegmentMaintenance(ctx context.Context, fin runs.Finish, operation string, maintenance func(context.Context) error) error {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx, "run segment maintenance",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			telemetry.RunIDKey.String(fin.RunID.String()),
			telemetry.SessionIDKey.String(fin.SessionID.String()),
			attribute.String("maintenance.operation", operation),
			attribute.Bool("run.parked", fin.Parked),
		),
	)
	defer span.End()
	err := maintenance(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (f *Finalizer) snapshot(ctx context.Context, sessionID identity.SessionID, cwd string, runID identity.RunID) error {
	if err := f.checkpoints.Snapshot(ctx, sessionID, cwd, runID); err != nil {
		return fmt.Errorf("segment: snapshot workspace for run %q: %w", runID, err)
	}
	return nil
}

func (f *Finalizer) title(ctx context.Context, claim *sessions.GeneratedTitle, sessionID identity.SessionID, prompt string) error {
	title, generationErr := f.titles.Generate(ctx, strings.TrimSpace(prompt))
	title = strings.TrimSpace(title)
	if generationErr != nil {
		generationErr = fmt.Errorf("segment: generate title for session %q: %w", sessionID, generationErr)
	}
	if title == "" {
		if generationErr == nil {
			return fmt.Errorf("segment: generated title for session %q is empty", sessionID)
		}
		return generationErr
	}
	if err := claim.Apply(ctx, title); err != nil {
		return errors.Join(generationErr, fmt.Errorf("segment: apply generated title to Session %q: %w", sessionID, err))
	}
	return generationErr
}
