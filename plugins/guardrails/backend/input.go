package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"

	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/Tangerg/scope/core/chat"
)

func inspectRequest(ctx context.Context, policy guardrail.Policy, request *chat.Request) error {
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(encoded))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// Matching decoded strings prevents JSON escaping from hiding protected text.
		if token.Kind() == '"' {
			if err := policy.Admit(token.String()); err != nil {
				return err
			}
		}
	}
}
