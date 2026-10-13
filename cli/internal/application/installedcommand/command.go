package installedcommand

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

type Caller interface {
	CallPlugin(context.Context, protocol.CallPluginRequest, replay.CommandID) (*protocol.CallPluginResponse, error)
}

// Execute stages the complete request before I/O. An unresolved intent always
// resumes its original request, including after restart or provider replacement.
func Execute(ctx context.Context, caller Caller, store *workbench.Store, intent string, policy mutation.ReplayPolicy, prepare func(context.Context) (protocol.CallPluginRequest, error)) error {
	if store == nil {
		return workbench.ErrUnavailable
	}
	pending, exists := store.PendingInstalledCommand(intent)
	if !exists {
		request, err := prepare(ctx)
		if err != nil {
			return err
		}
		guard, err := policy.NewGuard()
		if err != nil {
			return err
		}
		pending = workbench.PendingInstalledCommand{Intent: intent, CommandID: mutation.NewCommandID(), Request: request, Replay: guard}
		if err := store.StageInstalledCommand(pending); err != nil {
			return fmt.Errorf("stage installed command: %w", err)
		}
		pending, exists = store.PendingInstalledCommand(intent)
		if !exists {
			return errors.New("staged installed command is absent")
		}
	}
	_, err := mutation.ConfirmAdmitted(ctx, mutation.AcknowledgementBackoff(), mutation.ReplayAdmission(policy, pending.Replay), func(ctx context.Context) (*protocol.CallPluginResponse, error) {
		request := pending.Request
		request.Input = append(request.Input[:0:0], pending.Request.Input...)
		reply, err := caller.CallPlugin(ctx, request, pending.CommandID)
		if err == nil && reply == nil {
			return nil, errors.New("installed command acknowledgement is absent")
		}
		if err == nil && reply.Failure != nil {
			err = &capability.Refusal{Code: reply.Failure.Code, Message: reply.Failure.Message}
		}
		return reply, err
	})
	var refused *capability.Refusal
	if err != nil && !errors.As(err, &refused) {
		return fmt.Errorf("installed command outcome remains unknown: %w", errors.Join(replay.ErrCommandOutcomeUnknown, err))
	}
	if retireErr := store.RetireInstalledCommand(pending); retireErr != nil {
		return errors.Join(err, fmt.Errorf("retire installed command intent: %w", retireErr))
	}
	return err
}
