package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/plugins/usagereports/backend/domain"
	"github.com/Tangerg/flame/plugins/usagereports/backend/rpc"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
	"github.com/Tangerg/flame/runtime/protocol"
)

func capturedCoreSummary() *protocol.UsageSummary {
	observed := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	return &protocol.UsageSummary{
		ObservedAt: observed, Since: new(observed.AddDate(0, 0, -1)),
		RecordedUsage: protocol.RecordedUsage{Total: protocol.ModelUsage{InputTokens: 50}, RecordedCostUSD: new(float64(0)), Coverage: protocol.UsageCoverage{Calls: 2, ReportedCalls: 1, PricedCalls: 1, UnknownCalls: 1}},
		ByProvider:    []protocol.UsageBucket{}, ByModel: []protocol.UsageBucket{}, ByDay: []protocol.UsageBucket{}, ByOrigin: []protocol.UsageBucket{}, ByScope: []protocol.UsageBucket{},
	}
}

func TestCapturedAccountingIsVerbatimAndReceiptReplayNeverReadsOrRecreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.db")
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	summary := capturedCoreSummary()
	original, err := jsoninput.Encode(summary)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	read := func(_ context.Context, days int) (*protocol.UsageSummary, error) {
		calls++
		if days != 1 {
			t.Errorf("period was not passed to core: %d", days)
		}
		return summary, nil
	}
	archive, err := newArchive(store, read)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := rpc.NewContracts[protocol.UsageSummary]()
	if err != nil {
		t.Fatal(err)
	}
	input := rpc.CaptureInput{Period: domain.Daily}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	header, err := archive.capture(t.Context(), contracts.Capture.Name(), "save", input, admitted)
	if err != nil {
		t.Fatal(err)
	}
	summary.Total.InputTokens = 999
	stored, err := store.read(t.Context(), header.ID)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := jsoninput.Encode(stored.Summary)
	if err != nil || projected != original || stored.Summary.Total.CostUSD != nil || stored.Summary.RecordedCostUSD == nil || *stored.Summary.RecordedCostUSD != 0 {
		t.Fatalf("captured facts were recalculated: %s %v", projected.String(), err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	archive, err = newArchive(store, read)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := archive.capture(t.Context(), contracts.Capture.Name(), "save", input, admitted)
	if err != nil || !reflect.DeepEqual(replayed, header) || calls != 1 {
		t.Fatalf("replay changed original observation: %+v %v calls=%d", replayed, err, calls)
	}
	changed := rpc.CaptureInput{Period: domain.Weekly}
	changedInput, _ := jsoninput.Encode(changed)
	if _, err := archive.capture(t.Context(), contracts.Capture.Name(), "save", changed, changedInput); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("command identity changed input: %v", err)
	}
	remove := rpc.ReportInput{ID: header.ID}
	removeInput, _ := jsoninput.Encode(remove)
	deleteReport := func(ctx context.Context, input rpc.ReportInput) (bool, error) { return store.delete(ctx, input.ID) }
	if _, err := pluginstorage.Command(t.Context(), store.db, contracts.Delete.Name(), "delete", remove, removeInput, classify, deleteReport); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.capture(t.Context(), contracts.Capture.Name(), "save", input, admitted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.read(t.Context(), header.ID); !errors.Is(err, domain.ErrNotFound) || calls != 1 {
		t.Fatalf("capture replay recreated removed data: %v calls=%d", err, calls)
	}
	newHeader, err := archive.capture(t.Context(), contracts.Capture.Name(), "new-save", input, admitted)
	if err != nil || newHeader.ID == header.ID {
		t.Fatalf("new capture reused removed identity: %+v %v", newHeader, err)
	}
	if _, err := pluginstorage.Command(t.Context(), store.db, contracts.Delete.Name(), "delete", remove, removeInput, classify, deleteReport); err != nil {
		t.Fatal(err)
	}
	if _, err := store.read(t.Context(), newHeader.ID); err != nil {
		t.Fatalf("old removal advanced a new capture: %v", err)
	}
}

func TestMissingCaptureSettlementCannotAuthorizeAnotherCoreObservation(t *testing.T) {
	store, err := openStore(t.Context(), filepath.Join(t.TempDir(), "pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	cause := errors.New("core read interrupted")
	calls := 0
	archive, err := newArchive(store, func(context.Context, int) (*protocol.UsageSummary, error) { calls++; return nil, cause })
	if err != nil {
		t.Fatal(err)
	}
	contracts, _ := rpc.NewContracts[protocol.UsageSummary]()
	input := rpc.CaptureInput{Period: domain.Daily}
	admitted, _ := jsoninput.Encode(input)
	if _, err := archive.capture(t.Context(), contracts.Capture.Name(), "unknown", input, admitted); !errors.Is(err, cause) {
		t.Fatalf("unexpected read failure disappeared: %v", err)
	}
	if _, err := archive.capture(t.Context(), contracts.Capture.Name(), "unknown", input, admitted); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) || calls != 1 {
		t.Fatalf("missing receipt permitted another observation: %v calls=%d", err, calls)
	}
}
