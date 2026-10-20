package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const ReceiptMethod = "io.github.tangerg.flame/commands.get"

type ReceiptRequest struct {
	ID          string `json:"id"`
	Method      string `json:"method"`
	InputDigest string `json:"inputDigest"`
}

func (r *ReceiptRequest) UnmarshalJSON(data []byte) error {
	type wire ReceiptRequest
	var decoded wire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	request := ReceiptRequest(decoded)
	if err := request.Validate(); err != nil {
		return err
	}
	*r = request
	return nil
}

func (r ReceiptRequest) Validate() error {
	if err := validateCommandID(r.ID); err != nil {
		return err
	}
	if r.Method == "" || len(r.Method) > 256 || !utf8.ValidString(r.Method) {
		return errors.New("plugin: bounded receipt method identity is required")
	}
	digest, err := hex.DecodeString(r.InputDigest)
	if err != nil || len(digest) != sha256.Size || r.InputDigest != strings.ToLower(r.InputDigest) {
		return errors.New("plugin: canonical receipt input digest is required")
	}
	return nil
}

type Receipt struct {
	Found bool           `json:"found"`
	Reply jsontext.Value `json:"reply,omitzero"`
}

func DecodeReply[T any](data []byte) (T, error) {
	var result Result[T]
	var zero T
	if err := json.Unmarshal(data, &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	return result.value("plugin receipt")
}

// A missing receipt is only absence of evidence. It never authorizes a fresh
// mutation or proves that the original command had no effect.
func ReadReceipt[P, R any](ctx context.Context, session *sdk.ClientSession, id, method string, input P) (R, bool, error) {
	var zero R
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return zero, false, err
	}
	binding, err := receiptBinding(session)
	if err != nil {
		return zero, false, err
	}
	value, found, err := readReceipt(ctx, binding, id, method, admitted)
	if err != nil || !found {
		return zero, found, err
	}
	var typed R
	if err := json.Unmarshal(value, &typed, json.RejectUnknownMembers(true)); err != nil {
		return zero, false, err
	}
	return typed, true, nil
}

func receiptBinding(session *sdk.ClientSession) (*Binding, error) {
	method, err := NewMethod[ReceiptRequest, Receipt](MethodConfig{Name: ReceiptMethod, Kind: capability.Query, Target: "installation", Description: ReceiptMethod})
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("plugin: client session is required")
	}
	return &Binding{session: session, method: method.contract}, nil
}

func readReceipt(ctx context.Context, binding *Binding, id, method string, admitted jsoninput.Input) (jsontext.Value, bool, error) {
	request := ReceiptRequest{ID: id, Method: method, InputDigest: admitted.Digest()}
	if err := request.Validate(); err != nil {
		return nil, false, err
	}
	input, err := jsoninput.Encode(request)
	if err != nil {
		return nil, false, err
	}
	wire, err := binding.invoke(ctx, "", input, sdk.ParamsBase{})
	if err != nil {
		return nil, false, err
	}
	result, err := decodeBoundResult[Receipt](wire)
	if err != nil {
		return nil, false, err
	}
	receipt, err := result.value(ReceiptMethod)
	if err != nil {
		return nil, false, err
	}
	if receipt.Found != (len(receipt.Reply) != 0) {
		return nil, false, errors.New("plugin: malformed receipt evidence")
	}
	if !receipt.Found {
		return nil, false, nil
	}
	value, err := DecodeReply[jsontext.Value](receipt.Reply)
	return value, true, err
}
