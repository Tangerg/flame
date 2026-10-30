package workbench

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// MemoryCommands supplies the installed publisher boundary. Workbench owns the
// retained intent; the publisher still owns admission and the business outcome.
type MemoryCommands interface {
	InstalledCaller
	PrepareAdd(context.Context, memory.AddInput) (protocol.CallPluginRequest, error)
	DecodeAdd(*protocol.CallPluginResponse) (memory.AddResult, error)
	PrepareUpdate(context.Context, memory.UpdateInput) (protocol.CallPluginRequest, error)
	DecodeUpdate(string, *protocol.CallPluginResponse) (memory.Item, error)
	PrepareReview(context.Context, string, uint64, memory.ReviewInputDecision) (protocol.CallPluginRequest, error)
	PrepareDelete(context.Context, string, uint64) (protocol.CallPluginRequest, error)
}

const memoryAdditionIntent = "memory-add"

func memoryUpdateIntent(id string) string   { return "memory-update:" + id }
func memoryReviewIntent(id string) string   { return "memory-review:" + id }
func memoryDeletionIntent(id string) string { return "memory-delete:" + id }

func ExecuteMemoryAddition(ctx context.Context, caller MemoryCommands, store *Store, policy mutation.ReplayPolicy, desired memory.AddInput) (memory.AddResult, error) {
	return executeInstalledCommand(ctx, caller, store, memoryAdditionIntent, policy,
		func(ctx context.Context) (protocol.CallPluginRequest, error) { return caller.PrepareAdd(ctx, desired) },
		func(request protocol.CallPluginRequest) error { return retainedMemoryAddition(request, desired) },
		func(reply *protocol.CallPluginResponse) (memory.AddResult, error) {
			output, err := caller.DecodeAdd(reply)
			if err == nil && memory.AddInputScope(output.Item.Scope) != desired.Scope {
				return memory.AddResult{}, errors.New("memory addition acknowledgement changed target scope")
			}
			return output, err
		})
}

func ExecuteMemoryUpdate(ctx context.Context, caller MemoryCommands, store *Store, policy mutation.ReplayPolicy, desired memory.UpdateInput) (memory.Item, error) {
	return executeInstalledCommand(ctx, caller, store, memoryUpdateIntent(desired.ID), policy,
		func(ctx context.Context) (protocol.CallPluginRequest, error) {
			return caller.PrepareUpdate(ctx, desired)
		},
		func(request protocol.CallPluginRequest) error { return retainedMemoryUpdate(request, desired) },
		func(reply *protocol.CallPluginResponse) (memory.Item, error) {
			return caller.DecodeUpdate(desired.ID, reply)
		})
}

func ExecuteMemoryReview(ctx context.Context, caller MemoryCommands, store *Store, policy mutation.ReplayPolicy, id string, expectedRevision uint64, decision memory.ReviewInputDecision) error {
	_, err := executeInstalledCommand(ctx, caller, store, memoryReviewIntent(id), policy,
		func(ctx context.Context) (protocol.CallPluginRequest, error) {
			return caller.PrepareReview(ctx, id, expectedRevision, decision)
		},
		func(request protocol.CallPluginRequest) error {
			original, err := retainedMemoryReview(request, id, decision)
			if err == nil && original.ExpectedRevision != expectedRevision {
				return errors.New("pending agent memory review has another revision")
			}
			return err
		},
		func(*protocol.CallPluginResponse) (memory.Ack, error) {
			return memory.Ack{}, nil
		})
	return err
}

func ExecuteMemoryDeletion(ctx context.Context, caller MemoryCommands, store *Store, policy mutation.ReplayPolicy, id string, expectedRevision uint64) error {
	_, err := executeInstalledCommand(ctx, caller, store, memoryDeletionIntent(id), policy,
		func(ctx context.Context) (protocol.CallPluginRequest, error) {
			return caller.PrepareDelete(ctx, id, expectedRevision)
		},
		func(request protocol.CallPluginRequest) error {
			input, err := memoryCommandInput[memory.ItemInput](request, memory.DeleteV2Method)
			if err != nil {
				return err
			}
			if input.ID != id || input.ExpectedRevision != expectedRevision {
				return errors.New("pending agent memory deletion has another identity or revision")
			}
			return nil
		},
		func(*protocol.CallPluginResponse) (memory.Ack, error) {
			return memory.Ack{}, nil
		})
	return err
}

// Pending readouts expose the original user intent without handing delivery a
// decoder or a write entrance into the journal.
func (s *Store) PendingMemoryAddition() (memory.AddInput, bool, error) {
	pending, found := s.PendingInstalledCommand(memoryAdditionIntent)
	if !found {
		return memory.AddInput{}, false, nil
	}
	input, err := memoryCommandInput[memory.AddInput](pending.Request, memory.AddV2Method)
	return input, true, err
}

func (s *Store) PendingMemoryUpdate(identity string) (memory.UpdateInput, bool, error) {
	pending, found, err := s.pendingMemoryItemCommand(memory.UpdateV2Method, memoryUpdateIntent(""), identity)
	if err != nil || !found {
		return memory.UpdateInput{}, found, err
	}
	input, err := memoryCommandInput[memory.UpdateInput](pending.Request, memory.UpdateV2Method)
	if err != nil {
		return input, true, err
	}
	if input.ID != strings.TrimPrefix(pending.Intent, memoryUpdateIntent("")) || (input.Content == nil && input.Pinned == nil) {
		return input, true, errors.New("pending agent memory update identity or patch is invalid")
	}
	return input, true, nil
}

func (s *Store) PendingMemoryReview(identity string, decision memory.ReviewInputDecision) (memory.ReviewInput, bool, error) {
	pending, found, err := s.pendingMemoryItemCommand(memory.ReviewV2Method, memoryReviewIntent(""), identity)
	if err != nil || !found {
		return memory.ReviewInput{}, found, err
	}
	input, err := retainedMemoryReview(pending.Request, strings.TrimPrefix(pending.Intent, memoryReviewIntent("")), decision)
	return input, true, err
}

func (s *Store) PendingMemoryDeletion(identity string) (memory.ItemInput, bool, error) {
	pending, found, err := s.pendingMemoryItemCommand(memory.DeleteV2Method, memoryDeletionIntent(""), identity)
	if err != nil || !found {
		return memory.ItemInput{}, found, err
	}
	input, err := memoryCommandInput[memory.ItemInput](pending.Request, memory.DeleteV2Method)
	if err == nil && input.ID != strings.TrimPrefix(pending.Intent, memoryDeletionIntent("")) {
		err = errors.New("pending agent memory deletion has another identity")
	}
	return input, true, err
}

func (s *Store) pendingMemoryItemCommand(method, prefix, identity string) (PendingInstalledCommand, bool, error) {
	var matches []PendingInstalledCommand
	for _, pending := range s.PendingInstalledCommands() {
		id, found := strings.CutPrefix(pending.Intent, prefix)
		if !found || pending.Request.Method != method {
			continue
		}
		if id == identity {
			return pending, true, nil
		}
		if strings.HasPrefix(id, identity) {
			matches = append(matches, pending)
		}
	}
	switch len(matches) {
	case 0:
		return PendingInstalledCommand{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return PendingInstalledCommand{}, false, errors.New("pending agent memory command identity is ambiguous; use the full id")
	}
}

func memoryCommandInput[T any](request protocol.CallPluginRequest, method string) (T, error) {
	var input T
	if request.Method != method {
		return input, errors.New("pending agent memory command has another method")
	}
	if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
		return input, fmt.Errorf("pending agent memory input is invalid: %w", err)
	}
	return input, nil
}

func retainedMemoryReview(request protocol.CallPluginRequest, id string, decision memory.ReviewInputDecision) (memory.ReviewInput, error) {
	input, err := memoryCommandInput[memory.ReviewInput](request, memory.ReviewV2Method)
	if err != nil {
		return input, err
	}
	if input.ID != id || (input.Decision != memory.ReviewInputDecisionApprove && input.Decision != memory.ReviewInputDecisionReject) {
		return memory.ReviewInput{}, errors.New("pending agent memory review identity or decision is invalid")
	}
	if input.Decision != decision {
		return input, fmt.Errorf("agent memory %s has an unconfirmed %s review; reconcile the original command before changing its decision", id, input.Decision)
	}
	return input, nil
}

func retainedMemoryUpdate(request protocol.CallPluginRequest, desired memory.UpdateInput) error {
	original, err := memoryCommandInput[memory.UpdateInput](request, memory.UpdateV2Method)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(original, desired) {
		return fmt.Errorf("agent memory %s has an unconfirmed update; reconcile the original patch before submitting changes", desired.ID)
	}
	return nil
}

func retainedMemoryAddition(request protocol.CallPluginRequest, desired memory.AddInput) error {
	original, err := memoryCommandInput[memory.AddInput](request, memory.AddV2Method)
	if err != nil {
		return err
	}
	if original != desired {
		return errors.New("agent memory addition remains unconfirmed; reconcile the original target and content before submitting changes")
	}
	return nil
}
