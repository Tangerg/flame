package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/go/sqlite"
	"github.com/Tangerg/flame/plugins/usagereports/backend/domain"
	"github.com/Tangerg/flame/plugins/usagereports/backend/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

type accountingRead func(context.Context, int) (*protocol.UsageSummary, error)

func coreAccounting(ctx context.Context, days int) (summary *protocol.UsageSummary, err error) {
	ref, err := pluginsdk.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	client, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	return client.GetUsageSummary(ctx, protocol.UsageSummaryRequest{SinceDays: new(days)}, protocol.CallOptions{})
}

type archive struct {
	store *store
	read  accountingRead
}

func newArchive(store *store, read accountingRead) (*archive, error) {
	if store == nil || store.db == nil || read == nil {
		return nil, errors.New("usage reports: private archive and core accounting read are required")
	}
	return &archive{store: store, read: read}, nil
}

func (a *archive) capture(ctx context.Context, method, id string, input rpc.CaptureInput, original jsoninput.Input) (rpc.Header, error) {
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, a.store.db, method, id, original, nil)
	if err != nil {
		return rpc.Header{}, err
	}
	if prepared == nil {
		return pluginsdk.DecodeReply[rpc.Header](reply)
	}
	if !prepared.NewlyAdmitted() {
		return rpc.Header{}, pluginstorage.ErrCommandUnconfirmed
	}
	days, err := input.Period.Days()
	if err != nil {
		return a.store.completeCapture(ctx, prepared, domain.Report{}, err)
	}
	summary, err := a.read(ctx, days)
	if err != nil {
		return a.store.completeCapture(ctx, prepared, domain.Report{}, err)
	}
	if err := admitSummary(summary); err != nil {
		return a.store.completeCapture(ctx, prepared, domain.Report{}, err)
	}
	value, err := jsoninput.Encode(summary)
	if err != nil {
		return rpc.Header{}, err
	}
	report, err := domain.NewReport(id, input.Period, value)
	return a.store.completeCapture(ctx, prepared, report, err)
}

func admitSummary(summary *protocol.UsageSummary) error {
	if summary == nil || summary.ObservedAt.IsZero() || summary.Since == nil || summary.Since.IsZero() || summary.Since.After(summary.ObservedAt) {
		return errors.New("usage reports: core accounting must supply its actual observation time and accepted range")
	}
	return protocol.ValidateWireTree(summary)
}

func project(report domain.Report) (rpc.Report[protocol.UsageSummary], error) {
	snapshot := report.Snapshot()
	var summary protocol.UsageSummary
	if err := json.Unmarshal(snapshot.Value.JSON(), &summary, json.RejectUnknownMembers(true)); err != nil {
		return rpc.Report[protocol.UsageSummary]{}, err
	}
	if err := admitSummary(&summary); err != nil {
		return rpc.Report[protocol.UsageSummary]{}, err
	}
	return rpc.Report[protocol.UsageSummary]{Header: rpc.Header{ID: snapshot.ID, Period: snapshot.Period, ObservedAt: summary.ObservedAt, Since: new(*summary.Since)}, Summary: summary}, nil
}

func classify(err error) string {
	if errors.Is(err, errStoredReport) {
		return ""
	}
	return rpc.Classify(err)
}
