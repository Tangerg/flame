package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func newServer(db *sql.DB) (*sdk.Server, error) {
	if db == nil {
		return nil, errors.New("hooks: private command store is required")
	}
	contract, err := hookmcp.Contract()
	if err != nil {
		return nil, err
	}
	read, err := hookmcp.DiscoverMethod()
	if err != nil {
		return nil, err
	}
	execute, err := hookmcp.EvaluateMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.hooks", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		read.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[hookprovider.Discovery]) ([]hook.Hook, error) {
			return discover(ctx, command.Input)
		}),
		execute.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[hookprovider.Evaluation]) (hookprovider.Advice, error) {
			return evaluateEvent(ctx, db, execute.Name(), command)
		}),
		backend.BindCapability("hook-provider", contract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func evaluateEvent(ctx context.Context, db *sql.DB, method string, command pluginmcp.Command[hookprovider.Evaluation]) (hookprovider.Advice, error) {
	input, err := command.RequestInput()
	if err != nil {
		return hookprovider.Advice{}, err
	}
	receipt, reply, err := pluginstorage.AdmitCommand(ctx, db, method, string(command.ID), input, func(context.Context) error {
		if err := command.Input.Validate(); err != nil {
			return &framemcp.Refusal{Code: "invalid_input", Message: err.Error()}
		}
		return nil
	})
	if err != nil {
		return hookprovider.Advice{}, err
	}
	if len(reply) != 0 {
		return pluginmcp.DecodeReply[hookprovider.Advice](reply)
	}
	advice := hookprovider.Advice{Diagnostics: []hookprovider.Diagnostic{}}
	executor := &eventCommands{db: db, eventID: command.ID, reconcileOnly: !receipt.NewlyAdmitted()}
	rules := command.Input.Rules
	if command.Input.DeclarativeOnly {
		rules = slices.DeleteFunc(slices.Clone(rules), func(rule hook.Hook) bool { return rule.Command != "" })
	}
	evaluator := newPolicyEvaluator(executor, func(_ context.Context, source string, err error) {
		advice.Diagnostics = append(advice.Diagnostics, hookprovider.Diagnostic{Source: source, Message: err.Error()})
	})
	decision := evaluator.Run(ctx, rules, command.Input.Input)
	if executor.uncertain != nil {
		return hookprovider.Advice{}, executor.uncertain
	}
	advice.Decision = decision.CommandDecision
	if err := advice.Validate(); err != nil {
		return hookprovider.Advice{}, err
	}
	encoded, err := pluginmcp.EncodeReply(advice, nil, nil)
	if err != nil {
		return hookprovider.Advice{}, err
	}
	if err := receipt.Complete(context.WithoutCancel(ctx), encoded.Bytes()); err != nil {
		return hookprovider.Advice{}, err
	}
	return advice, nil
}

type eventCommands struct {
	reconcileOnly bool
	db            *sql.DB
	eventID       pluginmcp.CommandID
	uncertain     error
}

func (e *eventCommands) RunHookCommand(ctx context.Context, req commandRequest) commandResult {
	if e.uncertain != nil {
		return commandResult{Err: e.uncertain, ExitCode: -1}
	}
	request := hookprovider.Command{Command: req.Command, CWD: req.CWD, Input: req.Input, TimeoutMillis: req.Timeout.Milliseconds()}
	input, err := framevalue.EncodeJSON(request)
	if err != nil {
		e.uncertain = err
		return commandResult{Err: err, ExitCode: -1}
	}
	// A shell effect must retain its own identity even if the event's final reply is lost.
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", e.eventID, req.Ordinal))))
	result, err := e.rule(ctx, id, input)
	if err != nil {
		e.uncertain = err
		return commandResult{ExitCode: -1, Err: err}
	}
	out := commandResult{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Decision != nil {
		out.Decision = *result.Decision
	}
	if failure := result.Failure; failure != nil {
		out.Err = errors.New(failure.Message)
		out.TimedOut = failure.Kind == hookprovider.TimedOut
	}
	return out
}

const ruleCommandReceipt = "hooks.rule-command"

func (e *eventCommands) rule(ctx context.Context, id string, input framevalue.JSON) (hookprovider.Result, error) {
	if e.reconcileOnly {
		_, original, reply, err := pluginstorage.ResumeCommand(ctx, e.db, ruleCommandReceipt, id)
		if errors.Is(err, sql.ErrNoRows) {
			return hookprovider.Result{}, pluginstorage.ErrCommandUnconfirmed
		}
		if err != nil {
			return hookprovider.Result{}, err
		}
		if original != input {
			return hookprovider.Result{}, framemcp.ErrInvocationConflict
		}
		if len(reply) == 0 {
			return hookprovider.Result{}, pluginstorage.ErrCommandUnconfirmed
		}
		return pluginmcp.DecodeReply[hookprovider.Result](reply)
	}
	var request hookprovider.Command
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, e.db, ruleCommandReceipt, id, input, func(context.Context) error {
		if err := json.Unmarshal(input.Bytes(), &request, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if err := request.Validate(); err != nil {
			return &framemcp.Refusal{Code: "invalid_input", Message: err.Error()}
		}
		return nil
	})
	if err != nil {
		return hookprovider.Result{}, err
	}
	if len(reply) != 0 {
		return pluginmcp.DecodeReply[hookprovider.Result](reply)
	}
	if !prepared.NewlyAdmitted() {
		return hookprovider.Result{}, pluginstorage.ErrCommandUnconfirmed
	}
	result := (Shell{}).RunHookCommand(ctx, commandRequest{Command: request.Command, CWD: request.CWD, Input: request.Input, Timeout: time.Duration(request.TimeoutMillis) * time.Millisecond}).project()
	if err := result.Validate(); err != nil {
		return hookprovider.Result{}, err
	}
	encoded, err := pluginmcp.EncodeReply(result, nil, nil)
	if err != nil {
		return hookprovider.Result{}, err
	}
	if err := prepared.Complete(context.WithoutCancel(ctx), encoded.Bytes()); err != nil {
		return hookprovider.Result{}, err
	}
	return result, nil
}
