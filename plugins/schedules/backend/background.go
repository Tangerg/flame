package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framecall "github.com/Tangerg/frame/call"
	framesqlite "github.com/Tangerg/frame/sqlite"
	"github.com/google/uuid"
)

// scheduleScanner scans due schedules once a minute for its installation and
// resumes each captured Run through its original owner.
type scheduleScanner struct {
	contribution string
	db           framesqlite.Database
	store        store
	scanner      *app.Scanner
	host         runHost
	target       framecall.Target
}

func newBackground(ctx context.Context, db framesqlite.Database, store store, scanner *app.Scanner, host runHost, contribution string, target framecall.Target, closeHost func() error) (*pluginmcp.Background, error) {
	b := &scheduleScanner{contribution: contribution, db: db, store: store, scanner: scanner, host: host, target: target}
	return pluginmcp.NewBackground(ctx, pluginmcp.BackgroundConfig{Run: b.run, Release: closeHost})
}

func (b *scheduleScanner) run(ctx context.Context) {
	b.scan(ctx, time.Now())
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			b.scan(ctx, now)
		}
	}
}
func (b *scheduleScanner) scan(lifetime context.Context, now time.Time) {
	invocation := framecall.Invocation{ID: uuid.NewString(), Origin: extension.Automatic, Target: b.target}
	ctx, err := pluginmcp.WithInvocation(lifetime, invocation)
	if err != nil {
		report(lifetime, "caller admission", err)
		return
	}
	namespace, err := b.host.Namespace(ctx)
	if err == nil {
		err = pluginstorage.BindRuntimeTarget(ctx, b.db, namespace)
	}
	if err != nil {
		report(lifetime, "Runtime binding", err)
		return
	}
	requests, err := b.scanner.Scan(ctx, now, invocation)
	if err != nil {
		report(lifetime, "private scan", err)
		return
	}
	if len(requests) > 0 {
		defer func() {
			if err := b.host.Notify(ctx, b.contribution); err != nil {
				report(lifetime, "read invalidation", err)
			}
		}()
	}
	for _, request := range requests {
		if lifetime.Err() != nil {
			return
		}
		if request.Invocation().Target != b.target {
			report(lifetime, "captured target", errors.New("schedules: capture belongs to another installation"))
			continue
		}
		calling, err := pluginmcp.WithInvocation(lifetime, request.Invocation())
		if err == nil {
			_, err = resumeCaptured(calling, b.db, b.store, b.host, request)
		}
		if err != nil {
			report(lifetime, "original Run admission", err)
		}
	}
}
func report(lifetime context.Context, stage string, err error) {
	if lifetime.Err() == nil {
		slog.ErrorContext(lifetime, "schedules: background operation failed", "stage", stage, "error", err)
	}
}

func resumeCaptured(ctx context.Context, db framesqlite.Database, store store, host runHost, request schedule.RunRequest) (app.RunReference, error) {
	if err := request.Validate(); err != nil {
		return app.RunReference{}, err
	}
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	if invocation != request.Invocation() {
		return app.RunReference{}, errors.New("schedules: recovery caller differs from original capture")
	}
	dispatchStore := app.DispatchStore(store)
	if _, manual := request.ManualRecord(); manual {
		prepared, input, original, err := pluginstorage.ResumeCommand(ctx, db, wire.RunNow, request.CommandID())
		if err != nil {
			return app.RunReference{}, err
		}
		var item wire.ItemInput
		if err := json.Unmarshal(input.Bytes(), &item, json.RejectUnknownMembers(true)); err != nil {
			return app.RunReference{}, err
		}
		if item.ID != request.ScheduleID() {
			return app.RunReference{}, errors.New("schedules: original manual input differs from capture")
		}
		if prepared == nil {
			reference, err := pluginmcp.DecodeReply[app.RunReference](original)
			if err == nil && (reference.RunID != request.RunID() || reference.SessionID != request.SessionID()) {
				err = errors.New("schedules: original manual receipt differs from capture")
			}
			return reference, err
		}
		dispatchStore = dispatchCommit{DispatchStore: store, db: db, command: prepared}
	}
	dispatcher, err := app.NewDispatcher(dispatchStore, host)
	if err != nil {
		return app.RunReference{}, err
	}
	return dispatcher.Dispatch(ctx, request.RunID())
}
