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
func Execute[T any](ctx context.Context, caller Caller, store *workbench.Store, intent string, policy mutation.ReplayPolicy, prepare func(context.Context) (protocol.CallPluginRequest, error), decode func(*protocol.CallPluginResponse) (T, error)) (T, error) {
	var zero T
	if store == nil {
		return zero, workbench.ErrUnavailable
	}
	pending, exists := store.PendingInstalledCommand(intent)
	if !exists {
		request, err := prepare(ctx)
		if err != nil {
			return zero, err
		}
		guard, err := policy.NewGuard()
		if err != nil {
			return zero, err
		}
		pending = workbench.PendingInstalledCommand{Intent: intent, CommandID: mutation.NewCommandID(), Request: request, Replay: guard}
		if err := store.StageInstalledCommand(pending); err != nil {
			return zero, fmt.Errorf("stage installed command: %w", err)
		}
		pending, exists = store.PendingInstalledCommand(intent)
		if !exists {
			return zero, errors.New("staged installed command is absent")
		}
	}
	var ownerRefusal *capability.Refusal
	result, err := mutation.ConfirmAdmitted(ctx, mutation.AcknowledgementBackoff(), mutation.ReplayAdmission(policy, pending.Replay), func(ctx context.Context) (T, error) {
		request := pending.Request
		reply, err := caller.CallPlugin(ctx, request, pending.CommandID)
		ownerRefusal = nil
		if err != nil {
			errors.As(err, &ownerRefusal)
			return zero, err
		}
		if reply == nil {
			return zero, errors.New("installed command acknowledgement is absent")
		}
		if reply.Failure != nil {
			ownerRefusal = &capability.Refusal{Code: reply.Failure.Code, Message: reply.Failure.Message}
			return zero, ownerRefusal
		}
		return decode(reply)
	})
	if err != nil && ownerRefusal == nil {
		return zero, fmt.Errorf("installed command outcome remains unknown: %w", errors.Join(replay.ErrCommandOutcomeUnknown, err))
	}
	if retireErr := store.RetireInstalledCommand(pending); retireErr != nil {
		return zero, errors.Join(err, fmt.Errorf("retire installed command intent: %w", retireErr))
	}
	return result, err
}
