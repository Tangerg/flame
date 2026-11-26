package protocol

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/Tangerg/go-sdk/jsonrpc"
)

func DecodeRPCMessage(encoded []byte) (jsonrpc.Message, error) {
	if err := validateJSONRPCEnvelope(encoded); err != nil {
		return nil, err
	}
	message, err := jsonrpc.DecodeMessage(encoded)
	if err != nil {
		return nil, err
	}
	if request, ok := message.(*jsonrpc.Request); ok && request.Method == "" {
		return nil, errors.New("JSON-RPC request method is empty")
	}
	return message, nil
}

const (
	jsonRPCVersionMember = "jsonrpc"
	jsonRPCIDMember      = "id"
	jsonRPCMethodMember  = "method"
	jsonRPCParamsMember  = "params"
	jsonRPCResultMember  = "result"
	jsonRPCErrorMember   = "error"
)

// jsontext rejects duplicate members while skipping payload values, including
// nested objects. The envelope's ID is checked before the SDK can decode it.
func validateJSONRPCEnvelope(encoded []byte) error {
	decoder := jsontext.NewDecoder(bytes.NewReader(encoded))
	opening, err := decoder.ReadToken()
	if err != nil {
		return err
	}
	if opening.Kind() != jsontext.KindBeginObject {
		return errors.New("JSON-RPC message must be an object")
	}
	members := make(rpcMembers)
	for {
		token, err := decoder.ReadToken()
		if err != nil {
			return err
		}
		if token.Kind() == jsontext.KindEndObject {
			break
		}
		name := token.String()
		validID := name != jsonRPCIDMember || decoder.PeekKind() == jsontext.KindString
		if err := decoder.SkipValue(); err != nil {
			return err
		}
		if !validID {
			return errors.New("JSON-RPC id must be a string; omit id for a notification")
		}
		members[name] = true
	}
	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		return errors.New("JSON-RPC message carries more than one JSON value")
	}
	return members.validate()
}

type rpcMembers map[string]bool

func (members rpcMembers) validate() error {
	if members[jsonRPCMethodMember] {
		return members.rejectUnknown(
			"request",
			jsonRPCVersionMember,
			jsonRPCIDMember,
			jsonRPCMethodMember,
			jsonRPCParamsMember,
		)
	}
	hasResult := members[jsonRPCResultMember]
	hasError := members[jsonRPCErrorMember]
	if hasResult == hasError {
		if hasResult {
			return errors.New("JSON-RPC response contains both result and error")
		}
		return errors.New("JSON-RPC message contains neither method, result, nor error")
	}
	return members.rejectUnknown(
		"response",
		jsonRPCVersionMember,
		jsonRPCIDMember,
		jsonRPCResultMember,
		jsonRPCErrorMember,
	)
}

func (members rpcMembers) rejectUnknown(kind string, allowed ...string) error {
	for _, member := range slices.Sorted(maps.Keys(members)) {
		if !slices.Contains(allowed, member) {
			return fmt.Errorf("unknown JSON-RPC %s member %q", kind, member)
		}
	}
	return nil
}
