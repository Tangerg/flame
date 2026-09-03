package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
)

// BeginContinuation converts every validated application answer into an
// Interaction response Signal. It is called only after the fresh Segment
// opening commits, so accepting the Signal cannot start model/tool work ahead
// of the product's durable lifecycle boundary.
func (i *InteractionExecutor) BeginContinuation(
	ctx context.Context,
	ref runs.ExecutorRef,
	answers []runs.InterruptAnswer,
	input *runs.CommittedUserInput,
	allowedInterrupts []interrupt.Kind,
) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	if beginContinuationErr := session.beginContinuation(allowedInterrupts); beginContinuationErr != nil {
		return beginContinuationErr
	}
	paused, err := session.pausedProcessIDs()
	if err != nil {
		return fmt.Errorf("agentexec: inspect paused Interaction members: %w", err)
	}
	prepared, err := session.prepareContinuationAnswers(ctx, answers)
	if err != nil {
		return err
	}
	committedInput, err := session.prepareCommittedContinuationInput(input)
	if err != nil {
		return err
	}
	// The previous Agent Process lifetime includes the human wait. Reset the
	// Segment clock before any answer can make that Process runnable.
	session.segmentClock.start()
	if err := session.deliverContinuationAnswers(ctx, prepared, committedInput); err != nil {
		return err
	}
	if err := session.resumePausedProcesses(ctx, paused); err != nil {
		return err
	}
	session.continuationAccepted()
	return nil
}

type preparedInteractionAnswer struct {
	process *agent.Process
	signal  agent.SignalRequest
}

type preparedCommittedInteractionInput struct {
	processID agent.ProcessID
	itemID    string
	content   []transcript.ContentBlock
}

func (i *interactionSession) prepareContinuationAnswers(
	ctx context.Context,
	answers []runs.InterruptAnswer,
) ([]preparedInteractionAnswer, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	checkpointState, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode staged Interaction checkpoint: %w", err)
	}
	interruptions, err := i.pendingInterruptions(checkpointState.tree)
	if err != nil {
		return nil, err
	}
	orderedAnswers := slices.Clone(answers)
	slices.SortFunc(orderedAnswers, func(left, right runs.InterruptAnswer) int {
		if order := strings.Compare(left.MemberID, right.MemberID); order != 0 {
			return order
		}
		return strings.Compare(left.RequestID, right.RequestID)
	})
	if len(orderedAnswers) != len(interruptions) {
		return nil, fmt.Errorf(
			"agentexec: %d Interaction answers do not match %d pending inputs",
			len(orderedAnswers), len(interruptions),
		)
	}
	prepared := make([]preparedInteractionAnswer, 0, len(orderedAnswers))
	for index, answer := range orderedAnswers {
		expected := interruptions[index]
		if answer.MemberID != expected.MemberID || answer.RequestID != expected.RequestID {
			return nil, errors.New("agentexec: interrupt answer set differs from the staged Interaction inputs")
		}
		processID, err := agent.ParseProcessID(answer.MemberID)
		if err != nil {
			return nil, fmt.Errorf("agentexec: parse answered Interaction member: %w", err)
		}
		process, found := i.engine.Process(processID)
		if !found {
			return nil, errors.New("agentexec: answered Interaction member is unavailable")
		}
		pending, found, err := interaction.PendingToolInputFromProcess(ctx, process)
		if err != nil {
			return nil, fmt.Errorf("agentexec: inspect pending Interaction input: %w", err)
		}
		if !found || answer.RequestID != pending.WaitID().String() {
			return nil, errors.New("agentexec: interrupt answer does not address the active Interaction input")
		}
		response, err := interactioninput.EncodeResolution(answer.Resolution)
		if err != nil {
			return nil, err
		}
		signalID, err := interactionAnswerSignalID(answer, response)
		if err != nil {
			return nil, err
		}
		signal, err := pending.ResponseSignal(signalID, response)
		if err != nil {
			return nil, fmt.Errorf("agentexec: construct Interaction answer Signal: %w", err)
		}
		prepared = append(prepared, preparedInteractionAnswer{process: process, signal: signal})
	}
	return prepared, nil
}

func (i *interactionSession) deliverContinuationAnswers(
	ctx context.Context,
	answers []preparedInteractionAnswer,
	input *preparedCommittedInteractionInput,
) error {
	deliveryContext := runExecutionContext(ctx, i.scope, i.start)
	inputRetained := false
	if input != nil {
		i.state.mu.Lock()
		if i.state.pendingContinuation != nil {
			i.state.mu.Unlock()
			return errors.New("agentexec: committed continuation input is already pending")
		}
		i.state.pendingContinuation = &pendingInteractionContinuation{
			processID: input.processID,
			itemID:    input.itemID,
			content:   transcript.CloneContent(input.content),
		}
		i.state.mu.Unlock()
		defer func() {
			if !inputRetained {
				i.state.mu.Lock()
				if pending := i.state.pendingContinuation; pending != nil &&
					pending.processID == input.processID && pending.itemID == input.itemID {
					i.state.pendingContinuation = nil
				}
				i.state.mu.Unlock()
			}
		}()
	}
	for _, answer := range answers {
		accepted, err := answer.process.DeliverSignal(deliveryContext, answer.signal)
		if err != nil {
			return fmt.Errorf("agentexec: deliver Interaction answer Signal: %w", err)
		}
		if !accepted {
			return errors.New("agentexec: Interaction answer Signal was already accepted")
		}
	}
	inputRetained = input != nil
	return nil
}

func (i *interactionSession) prepareCommittedContinuationInput(
	input *runs.CommittedUserInput,
) (*preparedCommittedInteractionInput, error) {
	if input == nil {
		return nil, nil
	}
	if _, err := resourceid.ParseItem(input.ItemID); err != nil {
		return nil, fmt.Errorf("agentexec: committed continuation input: %w", err)
	}
	if _, err := runs.MaterializeUserMessage(input.Content); err != nil {
		return nil, fmt.Errorf("agentexec: materialize committed continuation input: %w", err)
	}
	process := i.state.processHandle()
	if process == nil {
		return nil, runs.ErrExecutorNotLive
	}
	return &preparedCommittedInteractionInput{
		processID: process.ID(), itemID: input.ItemID,
		content: transcript.CloneContent(input.Content),
	}, nil
}

// SubmitSteer queues one user message for the next Interaction safe boundary.
// Agent Framework rejects it while the Process is waiting; accepted content is
// projected immediately before the model request that can first observe it.
func (i *InteractionExecutor) SubmitSteer(
	ctx context.Context,
	ref runs.ExecutorRef,
	input []transcript.ContentBlock,
) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	message, err := runs.MaterializeUserMessage(input)
	if err != nil {
		return err
	}
	return session.submitSteer(ctx, message, input)
}

func interactionAnswerSignalID(
	answer runs.InterruptAnswer,
	response json.RawMessage,
) (agent.SignalID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(answer.InterruptItemID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.MemberID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.RequestID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(response)
	return agent.ParseSignalID("answer:" + hex.EncodeToString(digest.Sum(nil)))
}
