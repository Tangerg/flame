package feedback

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/annotation"

	"github.com/samber/lo"
)

type preparer interface {
	PrepareFeedback(context.Context, extension.FeedbackDraft) (extension.AnnotationSource, framevalue.JSON, error)
}
type ledger interface {
	Append(context.Context, annotation.Entry) error
}

type Recorder struct {
	source preparer
	ledger ledger
	now    func() time.Time
}

func NewRecorder(source preparer, annotations ledger, now func() time.Time) (*Recorder, error) {
	if lo.IsNil(source) || lo.IsNil(annotations) || now == nil {
		return nil, errors.New("feedback: provider, annotation ledger and clock are required")
	}
	return &Recorder{source: source, ledger: annotations, now: now}, nil
}
func (r *Recorder) Record(ctx context.Context, in extension.FeedbackDraft) error {
	write, err := r.Prepare(ctx, in)
	if err != nil {
		return err
	}
	return write(ctx)
}

func (r *Recorder) Prepare(ctx context.Context, in extension.FeedbackDraft) (func(context.Context) error, error) {
	if operation.Identity(ctx) == "" {
		ctx = operation.Bind(ctx, "feedback.create", "")
	}
	source, content, err := r.source.PrepareFeedback(ctx, in)
	if err != nil {
		return nil, err
	}
	note := annotation.Entry{CommandID: operation.Identity(ctx), Source: source, Subject: annotation.Subject{SessionID: in.SessionID, RunID: in.RunID, ItemID: in.ItemID}, Content: content, CreatedAt: r.now().UTC()}
	return func(tx context.Context) error { return r.ledger.Append(tx, note) }, nil
}
