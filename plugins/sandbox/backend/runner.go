package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"runtime"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/sqlite"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	processsandbox "github.com/Tangerg/flame/packages/go-sdk/sandbox"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	toolshell "github.com/Tangerg/scope/tools/shell"
)

type homeRead func(context.Context) (string, error)

func coreHome(ctx context.Context) (home string, err error) {
	ref, err := qimumcp.CallbackFromContext(ctx)
	if err != nil {
		return "", err
	}
	client, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	discovery, err := client.Discover(ctx, protocol.CallOptions{})
	if err != nil {
		return "", err
	}
	return discovery.ServerInfo.Home, nil
}

type runner struct {
	db      *sql.DB
	scratch string
	home    homeRead
}

func newRunner(db *sql.DB, scratch string, home homeRead) (*runner, error) {
	if db == nil || !filepath.IsAbs(scratch) || home == nil {
		return nil, errors.New("sandbox: private database, absolute scratch directory and core home read are required")
	}
	return &runner{db: db, scratch: scratch, home: home}, nil
}

func (r *runner) limits() wire.Limits {
	return wire.Limits{Available: processsandbox.Available(), Platform: runtime.GOOS, MaxScriptBytes: sandbox.MaxScriptBytes, MaxTimeoutMillis: sandbox.MaxTimeoutMillis, RetainedBytesPerStream: processsandbox.MaxCommandOutputBytes}
}

func (r *runner) execute(ctx context.Context, method, id string, original jsonvalue.Value) (sandbox.Result, error) {
	var input wire.ExecuteInput
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, r.db, method, id, original, func(context.Context) error {
		return json.Unmarshal(original.JSON(), &input, json.RejectUnknownMembers(true), jsonvalue.Numbers())
	})
	if err != nil {
		return sandbox.Result{}, err
	}
	if prepared == nil {
		return readOutcome(reply)
	}
	if !prepared.NewlyAdmitted() {
		return sandbox.Result{}, pluginstorage.ErrCommandUnconfirmed
	}
	program, err := sandbox.NewProgram(input.Script, input.TimeoutMillis)
	if err != nil {
		return r.complete(ctx, prepared, sandbox.Result{}, err)
	}
	result, err := r.run(ctx, program)
	return r.complete(ctx, prepared, result, err)
}

func (r *runner) run(ctx context.Context, program sandbox.Program) (result sandbox.Result, err error) {
	if !processsandbox.Available() {
		return sandbox.Result{}, processsandbox.ErrUnavailable
	}
	home, err := r.home(ctx)
	if err != nil {
		return sandbox.Result{}, err
	}
	workspace, err := processsandbox.New(ctx, processsandbox.Config{BaseDir: r.scratch, UserHome: home}, "")
	if err != nil {
		return sandbox.Result{}, err
	}
	defer func() { err = errors.Join(err, workspace.Shutdown()) }()
	limited, cancel := context.WithTimeout(ctx, program.Timeout())
	defer cancel()
	output, err := workspace.Run(limited, toolshell.Input{Cmd: program.Script()})
	if err != nil {
		return sandbox.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.Result{}, err
	}
	status := sandbox.Completed
	if output.CancellationObserved && errors.Is(limited.Err(), context.DeadlineExceeded) {
		status = sandbox.TimedOut
	}
	return sandbox.Result{Status: status, ExitCode: output.ExitCode, DurationMillis: output.Duration.Milliseconds(), Stdout: sandbox.Capture(output.Stdout), Stderr: sandbox.Capture(output.Stderr)}, nil
}

func (r *runner) complete(ctx context.Context, prepared *pluginstorage.PreparedCommand, result sandbox.Result, operationErr error) (sandbox.Result, error) {
	if operationErr == nil {
		if err := result.Validate(); err != nil {
			return sandbox.Result{}, err
		}
	}
	reply, err := qimumcp.EncodeReply(result, operationErr, classify)
	if err != nil {
		return sandbox.Result{}, err
	}
	encoded := reply.JSON()
	if err := prepared.Complete(ctx, encoded); err != nil {
		return sandbox.Result{}, err
	}
	return readOutcome(encoded)
}

func readOutcome(encoded []byte) (sandbox.Result, error) {
	result, err := qimumcp.DecodeReply[sandbox.Result](encoded)
	if err != nil {
		return sandbox.Result{}, err
	}
	return result, result.Validate()
}

func openStore(ctx context.Context, path string) (*sql.DB, error) {
	return sqlite.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema})
}
