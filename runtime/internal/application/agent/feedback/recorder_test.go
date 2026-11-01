package feedback

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/annotation"
	"github.com/Tangerg/qimu/jsonvalue"
)

type prepared struct {
	source  extension.AnnotationSource
	content jsonvalue.Value
	err     error
}

func (p prepared) PrepareFeedback(context.Context, extension.FeedbackDraft) (extension.AnnotationSource, jsonvalue.Value, error) {
	return p.source, p.content, p.err
}

type observed []annotation.Entry

func (o *observed) Append(_ context.Context, e annotation.Entry) error {
	*o = append(*o, e)
	return nil
}

func TestOnlyOriginalProviderCanSupplyObservation(t *testing.T) {
	src := extension.AnnotationSource{ContributionSource: extension.ContributionSource{InstallationID: "installed", Release: "original", Contribution: "observer"}, ContractReference: "contract"}
	content, _ := jsonvalue.Parse([]byte(`{"future":"meaning"}`))
	var ledger observed
	at := time.Now().UTC()
	ctx := operation.Bind(t.Context(), "feedback.create", "retry")
	recorder, err := NewRecorder(prepared{source: src, content: content}, &ledger, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(ctx, extension.FeedbackDraft{ItemID: "item_1"}); err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 || ledger[0].Source != src || ledger[0].Content != content || ledger[0].CreatedAt != at || ledger[0].CommandID != operation.Identity(ctx) || ledger[0].Subject.ItemID != "item_1" {
		t.Fatalf("altered provider evidence: %+v", ledger)
	}
	failure := errors.New("original source unavailable")
	recorder, _ = NewRecorder(prepared{err: failure}, &ledger, time.Now)
	if err := recorder.Record(ctx, extension.FeedbackDraft{}); !errors.Is(err, failure) || len(ledger) != 1 {
		t.Fatalf("unavailable source advanced observations: %v", err)
	}
}
