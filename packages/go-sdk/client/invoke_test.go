package client

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// A contract that cannot be parsed, or a valid contract missing the named
// method, is a caller error. invocationOperation runs before any transport and
// feeds both CallPlugin and StreamPlugin, so it must keep the public
// invalid_params identity the local handler also uses; otherwise callers cannot
// branch on it with errors.Is/As.
func TestInvocationOperationKeepsInvalidParamsIdentityBeforeTransport(t *testing.T) {
	meta := methodMeta{Operation: "extension"}
	if _, err := invocationOperation(meta, protocol.CallPluginRequest{Contract: "{not a contract", Method: "any"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("malformed contract lost its invalid_params identity: %v", err)
	}
	if _, err := invocationOperation(meta, protocol.CallPluginRequest{Contract: "not json at all", Method: ""}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("unparsable contract lost its invalid_params identity: %v", err)
	}
}
