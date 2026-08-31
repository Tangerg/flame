package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// Resolver selects the current utility-role client for each call. Resolving at
// the boundary lets a role configuration change take effect without rebuilding
// the owning worker.
type AuxiliaryResolver func(context.Context) *chatclient.Client

// callTimeout bounds one auxiliary model request independently of an Agent Run.
const callTimeout = 2 * time.Minute

// Prompt is the complete resource envelope for one auxiliary model request.
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

// Complete performs one synchronous, middleware-free prompt completion inside
// the caller's explicit input/output envelope.
func CompleteAuxiliary(ctx context.Context, client *chatclient.Client, prompt AuxiliaryPrompt) (string, error) {
	if client == nil {
		return "", errors.New("auxiliary model: client is required")
	}
	if err := prompt.validate(); err != nil {
		return "", err
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	response, err := client.Call(callCtx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxTokens: &prompt.MaxOutputTokens}})
	if err != nil {
		return "", err
	}
	return response.Text(), nil
}
