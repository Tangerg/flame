package modelprovider

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	coreinvocation "github.com/Tangerg/scope/core/invocation"
)

type StreamReceipt struct {
	ResponseDigest framevalue.Digest `json:"responseDigest,omitzero"`
	Failure        *Failure          `json:"failure,omitzero"`
}

func NewStreamReceipt(response *chat.Response, failure *Failure) (StreamReceipt, error) {
	var receipt StreamReceipt
	if failure != nil {
		if err := failure.Validate(); err != nil {
			return StreamReceipt{}, err
		}
		if response != nil && failure.Execution == coreinvocation.FailureNotExecuted {
			return StreamReceipt{}, errors.New("model provider: completed stream reported non-execution")
		}
		receipt.Failure = new(*failure)
	} else if response == nil {
		return StreamReceipt{}, errors.New("model provider: missing stream outcome")
	}
	if response != nil {
		encoded, err := framevalue.EncodeJSON(scopewire.Response(*response))
		if err != nil {
			return StreamReceipt{}, err
		}
		receipt.ResponseDigest = encoded.Digest()
	}
	return receipt, nil
}

func (r StreamReceipt) VerifyResponse(response *chat.Response) error {
	expected, err := NewStreamReceipt(response, r.Failure)
	if err != nil {
		return err
	}
	if expected.ResponseDigest != r.ResponseDigest {
		return errors.New("model provider: stream receipt disagrees with delivered response")
	}
	return nil
}
