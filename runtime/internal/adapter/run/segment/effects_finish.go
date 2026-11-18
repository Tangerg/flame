package segment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type sessionTitles interface {
	PrepareGeneratedTitle(ctx context.Context, sessionID, runID string) (*sessions.GeneratedTitle, error)
}

// titleGenerator derives an initial session title from its opening request. It
// may return a usable deterministic fallback with a non-nil error so maintenance
// can persist stable identity and still report provider degradation.
type titleGenerator interface {
	Generate(ctx context.Context, firstMessage string) (string, error)
}

// checkpoints anchors the working tree at a terminal run boundary.
type checkpoints interface {
	Snapshot(ctx context.Context, sessionID, cwd, runID string) error
}

// taskLauncher starts request-detached work owned by its component lifecycle.
type taskLauncher interface {
	Start(parent context.Context, task func(context.Context)) bool
}

// TitleMaintenance is the generated-title capability.
type TitleMaintenance struct {
	Sessions  sessionTitles
	Generator titleGenerator
	Tasks     taskLauncher
}

// FinalizerConfig declares the two post-segment maintenance features. A
// Runtime without file checkpoints supplies checkpoints that snapshot nothing.
type FinalizerConfig struct {
	Checkpoints checkpoints
	Titles      TitleMaintenance
}

// Finalizer owns post-boundary maintenance. It is deliberately separate from
// durable Effects because neither title generation nor workspace snapshots
// participate in the Run transaction.
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
		{"title generator", cfg.Titles.Generator},
		{"task launcher", cfg.Titles.Tasks},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("segment: %s are required", required.name)
		}
	}
	return &Finalizer{
		checkpoints:   cfg.Checkpoints,
		sessionTitles: cfg.Titles.Sessions,
		titles:        cfg.Titles.Generator,
		tasks:         cfg.Titles.Tasks,
	}, nil
}

// Finish establishes the terminal file boundary before returning, then starts
// title generation off the live path. The checkpoint is a sequencing fence: the
// run admission remains held by the caller until it completes, so a following
// run cannot write into the preceding run's snapshot. Title generation does not
// define the file boundary and may continue asynchronously. A parked Run skips
// the checkpoint, but it can still title its Session from the immutable opening
// user text so a waiting conversation remains discoverable after process exit.
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
		if err := observeSegmentMaintenance(ctx, fin, "checkpoint", func(ctx context.Context) error {
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
		// The async title error has no caller to return to; without this the
		// span is its only report and reaches no operator.
		if err := observeSegmentMaintenance(ctx, fin, "title", func(ctx context.Context) error {
			return f.title(ctx, claim, fin.SessionID, fin.OpeningUserText)
		}); err != nil {
			slog.WarnContext(ctx, "segment: maintain session title", "run.id", fin.RunID, "error", err)
		}
	}) {
		claim.Release()
		rejected := fmt.Errorf("segment: maintenance for run %q was rejected during shutdown", fin.RunID)
		errs = append(errs, observeSegmentMaintenance(ctx, fin, "title", func(context.Context) error { return rejected }))
	}
	return errors.Join(errs...)
}

func observeSegmentMaintenance(ctx context.Context, fin runs.Finish, operation string, maintenance func(context.Context) error) error {
	ctx, span := otel.Tracer(runsegmentTracerName).Start(ctx, "run segment maintenance",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("run.id", fin.RunID),
			attribute.String("gen_ai.conversation.id", fin.SessionID),
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

func (f *Finalizer) snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := f.checkpoints.Snapshot(ctx, sessionID, cwd, runID); err != nil {
		return fmt.Errorf("segment: snapshot workspace for run %q: %w", runID, err)
	}
	return nil
}

func (f *Finalizer) title(ctx context.Context, claim *sessions.GeneratedTitle, sessionID, prompt string) error {
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
