package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// AuxiliaryResolver selects the current utility-role client for each call.
// Resolving at the boundary lets a role configuration change take effect without
// rebuilding the owning worker.
type AuxiliaryResolver func(context.Context) (*chatclient.Client, error)

// callTimeout bounds one auxiliary model request independently of an Agent Run.
const callTimeout = 2 * time.Minute

// AuxiliaryPrompt is the complete resource envelope for one auxiliary model request.
// Input bytes and output tokens are deliberately mandatory: background
// maintenance must never inherit a provider's context/output defaults.
type AuxiliaryPrompt struct {
	SystemPrompt    string
	UserPrompt      string
	MaxInputBytes   int
	MaxOutputTokens int64
}

func (p AuxiliaryPrompt) validate() error {
	if p.MaxInputBytes <= 0 {
		return errors.New("auxiliary model: max input bytes must be positive")
	}
	if p.MaxOutputTokens <= 0 {
		return errors.New("auxiliary model: max output tokens must be positive")
	}
	inputBytes := len(p.SystemPrompt) + len(p.UserPrompt)
	if inputBytes > p.MaxInputBytes {
		return fmt.Errorf(
			"auxiliary model: prompt is %d bytes; input limit is %d",
			inputBytes,
			p.MaxInputBytes,
		)
	}
	return nil
}

// Complete resolves the live auxiliary selection and returns a complete text
// generation inside the caller's resource envelope. A rejected or incomplete
// generation cannot become a durable summary, memory, or skill proposal.
func (r AuxiliaryResolver) Complete(ctx context.Context, prompt AuxiliaryPrompt) (string, error) {
	if r == nil {
		return "", errors.New("auxiliary model: resolver is required")
	}
	if err := prompt.validate(); err != nil {
		return "", err
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	client, err := r(callCtx)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", errors.New("auxiliary model: client is required")
	}
	response, err := client.Call(callCtx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}})
	if err != nil {
		return "", err
	}
	if err := response.Validate(); err != nil {
		return "", fmt.Errorf("auxiliary model: invalid response: %w", err)
	}
	if response.Output.FinishReason != chat.FinishReasonStop {
		return "", fmt.Errorf("auxiliary model: generation did not complete (finish reason %q)", response.Output.FinishReason)
	}
	text := response.Text()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf(
			"auxiliary model: completed without text (finish reason %q)",
			response.Output.FinishReason,
		)
	}
	return text, nil
}
