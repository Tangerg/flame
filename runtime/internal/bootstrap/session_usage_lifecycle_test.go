package bootstrap

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProtocolSessionUsageKeepsCompleteProviderModelIdentities(t *testing.T) {
	t.Setenv("FLAME_HOME", t.TempDir())
	host, api, stores := openProtocolRuntimeWithStores(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}
	created := endpoint.Invoke(t.Context(), delivery.SessionsCreate, protocol.CreateSessionRequest{}, options)
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	sessionID := created.Value.(*protocol.Session).ID
	model := strings.Repeat("模", modelref.MaximumModelIdentityCharacters)
	providers := []string{
		strings.Repeat("a", modelref.MaximumProviderIdentityCharacters),
		strings.Repeat("o", modelref.MaximumProviderIdentityCharacters),
	}
	now := time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)
	for index, provider := range providers {
		selection, err := modelref.New(provider, model)
		if err != nil {
			t.Fatal(err)
		}
		value := testsupport.MustRestoreRun(run.Snapshot{
			ID: fmt.Sprintf("run_usage_%d", index), SessionID: sessionID, ModelSelection: selection,
			Outcome: new(run.OutcomeCompleted), CreatedAt: now, FinishedAt: now, UpdatedAt: now,
			Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{Usage: &accounting.Usage{
				Total:   accounting.Totals{InputTokens: 10},
				ByModel: map[string]accounting.Totals{model: {InputTokens: 10}},
			}}),
		})
		if err := stores.Runs.Restore(t.Context(), value); err != nil {
			t.Fatal(err)
		}
	}
	read := endpoint.Invoke(t.Context(), delivery.UsageSession, protocol.SessionUsageRequest{SessionID: sessionID}, options)
	if read.Failure != nil {
		t.Fatal(read.Failure)
	}
	usage := read.Value.(*protocol.SessionUsage)
	if usage.InputTokens != 20 || len(usage.ByModel) != 2 {
		t.Fatalf("Session usage = %+v", usage)
	}
	for index, bucket := range usage.ByModel {
		if bucket.Key != providers[index]+"/"+model || bucket.InputTokens != 10 || bucket.Runs != 1 {
			t.Fatalf("model bucket lost its identity or contribution: %+v", bucket)
		}
	}
	summaryRead := endpoint.Invoke(t.Context(), delivery.UsageSummary, protocol.UsageSummaryRequest{}, options)
	if summaryRead.Failure != nil {
		t.Fatal(summaryRead.Failure)
	}
	summary := summaryRead.Value.(*protocol.UsageSummary)
	if !reflect.DeepEqual(usage.ByModel, summary.ByModel) {
		t.Fatalf("Session and summary use different model accounting: %+v / %+v", usage.ByModel, summary.ByModel)
	}
}
