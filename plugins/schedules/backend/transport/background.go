package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/backend/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	"github.com/google/uuid"
)

type background struct {
	contribution    string
	closeDone       chan struct{}
	closeErr        error
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	done            chan struct{}
	started, closed bool
	db              *sql.DB
	store           Store
	scanner         *app.Scanner
	host            RunHost
	target          capability.Target
	closeHost       func() error
}

func newBackground(ctx context.Context, db *sql.DB, store Store, scanner *app.Scanner, host RunHost, contribution string, target capability.Target, closeHost func() error) *background {
	lifetime, cancel := context.WithCancel(ctx)
	return &background{contribution: contribution, closeDone: make(chan struct{}), ctx: lifetime, cancel: cancel, done: make(chan struct{}), db: db, store: store, scanner: scanner, host: host, target: target, closeHost: closeHost}
}
func (b *background) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.ctx.Err() != nil {
		return errors.New("schedules: background lifetime is closed")
	}
	if b.started {
		return nil
	}
	b.started = true
	go b.run()
	return nil
}
func (b *background) Close() error {
	b.cancel()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		<-b.closeDone
		return b.closeErr
	}
	b.closed = true
	if !b.started {
		close(b.done)
	}
	b.mu.Unlock()
	<-b.done
	b.closeErr = b.closeHost()
	close(b.closeDone)
	return b.closeErr
}
func (b *background) run() {
	defer close(b.done)
	b.scan(time.Now())
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case now := <-timer.C:
			b.scan(now)
		}
	}
}
func (b *background) scan(now time.Time) {
	invocation := capability.Invocation{ID: uuid.NewString(), Origin: capability.Automatic, Target: b.target}
	ctx, err := pluginsdk.WithInvocation(b.ctx, invocation)
	if err != nil {
		b.report("caller admission", err)
		return
	}
	namespace, err := b.host.Namespace(ctx)
	if err == nil {
		err = pluginstorage.BindRuntimeTarget(ctx, b.db, namespace)
	}
	if err != nil {
		b.report("Runtime binding", err)
		return
	}
	requests, err := b.scanner.Scan(ctx, now, invocation)
	if err != nil {
		b.report("private scan", err)
		return
	}
	if len(requests) > 0 {
		defer func() {
			if err := b.host.Notify(ctx, b.contribution); err != nil {
				b.report("read invalidation", err)
			}
		}()
	}
	for _, request := range requests {
		if b.ctx.Err() != nil {
			return
		}
		if request.Invocation().Target != b.target {
			b.report("captured target", errors.New("schedules: capture belongs to another installation"))
			continue
		}
		calling, err := pluginsdk.WithInvocation(b.ctx, request.Invocation())
		if err == nil {
			_, err = resumeCaptured(calling, b.db, b.store, b.host, request)
		}
		if err != nil {
			b.report("original Run admission", err)
		}
	}
}
func (b *background) report(stage string, err error) {
	if b.ctx.Err() == nil {
		slog.ErrorContext(b.ctx, "schedules: background operation failed", "stage", stage, "error", err)
	}
}

func resumeCaptured(ctx context.Context, db *sql.DB, store Store, host RunHost, request schedule.RunRequest) (app.RunReference, error) {
	if err := request.Validate(); err != nil {
		return app.RunReference{}, err
	}
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	if invocation != request.Invocation() {
		return app.RunReference{}, errors.New("schedules: recovery caller differs from original capture")
	}
	dispatchStore := app.DispatchStore(store)
	if _, manual := request.ManualRecord(); manual {
		prepared, input, original, err := pluginstorage.ResumeCommand(ctx, db, rpc.RunNow, request.Invocation().ID)
		if err != nil {
			return app.RunReference{}, err
		}
		var item rpc.ItemInput
		if err := json.Unmarshal(input.JSON(), &item, json.RejectUnknownMembers(true)); err != nil {
			return app.RunReference{}, err
		}
		if item.ID != request.ScheduleID() {
			return app.RunReference{}, errors.New("schedules: original manual input differs from capture")
		}
		if prepared == nil {
			reference, err := pluginsdk.DecodeReply[app.RunReference](original)
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
