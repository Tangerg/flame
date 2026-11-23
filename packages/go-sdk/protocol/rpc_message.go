package protocol

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/go-sdk/jsonrpc"
)

// Duplicate members are rejected so one byte sequence cannot be read as two
// different requests by first-wins and last-wins decoders.
func DecodeRPCMessage(encoded []byte) (jsonrpc.Message, error) {
	if err := validateSingleUnambiguousValue(encoded); err != nil {
		return nil, err
	}
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

// Tokens are walked rather than decoded so an id past IEEE-754's exact range is
// never read through float64.
func validateJSONRPCEnvelope(encoded []byte) error {
	parser := jsonEnvelopeParser{decoder: jsontext.NewDecoder(bytes.NewReader(encoded))}
	if _, err := parser.value(envelopeLevel); err != nil {
		return err
	}
	return nil
}

type jsonValueKind uint8

const (
	jsonString jsonValueKind = iota
	jsonNumber
	jsonBoolean
	jsonNull
	jsonObject
	jsonArray
)

type jsonEnvelopeParser struct {
	decoder *jsontext.Decoder
}

// Only the outermost object is the JSON-RPC envelope; nested objects are payload.
type jsonLevel uint8

const (
	envelopeLevel jsonLevel = iota
	payloadLevel
)

func (parser jsonEnvelopeParser) value(level jsonLevel) (jsonValueKind, error) {
	token, err := parser.decoder.ReadToken()
	if err != nil {
		return jsonNull, err
	}
	switch token.Kind() {
	case '{':
		return parser.object(level)
	case '[':
		return parser.array()
	case '"':
		return jsonString, nil
	case '0':
		return jsonNumber, nil
	case 't', 'f':
		return jsonBoolean, nil
	case 'n':
		return jsonNull, nil
	default:
		return jsonNull, fmt.Errorf("unexpected JSON token %q", token.Kind())
	}
}

func (parser jsonEnvelopeParser) object(level jsonLevel) (jsonValueKind, error) {
	members := make(jsonObjectMembers)
	// PeekKind reports KindInvalid once the input cannot yield another token, so
	// a truncated object leaves the loop and is reported by close rather than
	// spinning here.
	for parser.decoder.PeekKind() != '}' && parser.decoder.PeekKind() != jsontext.KindInvalid {
		member, err := parser.memberName()
		if err != nil {
			return jsonObject, err
		}
		members.add(member)
		valueKind, err := parser.value(payloadLevel)
		if err != nil {
			return jsonObject, err
		}
		if level == envelopeLevel && member == jsonRPCIDMember && valueKind != jsonString {
			return jsonObject, errors.New(
				"JSON-RPC id must be a string; omit id for a notification",
			)
		}
	}
	if err := parser.close('}', "JSON object is not closed"); err != nil {
		return jsonObject, err
	}
	if level == envelopeLevel {
		if err := members.validateEnvelope(); err != nil {
			return jsonObject, err
		}
	}
	return jsonObject, nil
}

func (parser jsonEnvelopeParser) memberName() (string, error) {
	token, err := parser.decoder.ReadToken()
	if err != nil {
		return "", err
	}
	if token.Kind() != '"' {
		return "", errors.New("JSON object member name is not a string")
	}
	return token.String(), nil
}

func (parser jsonEnvelopeParser) array() (jsonValueKind, error) {
	for parser.decoder.PeekKind() != ']' && parser.decoder.PeekKind() != jsontext.KindInvalid {
		if _, err := parser.value(payloadLevel); err != nil {
			return jsonArray, err
		}
	}
	if err := parser.close(']', "JSON array is not closed"); err != nil {
		return jsonArray, err
	}
	return jsonArray, nil
}

func (parser jsonEnvelopeParser) close(expected jsontext.Kind, message string) error {
	closing, err := parser.decoder.ReadToken()
	if err != nil {
		return err
	}
	if closing.Kind() != expected {
		return errors.New(message)
	}
	return nil
}

const (
	jsonRPCVersionMember = "jsonrpc"
	jsonRPCIDMember      = "id"
	jsonRPCMethodMember  = "method"
	jsonRPCParamsMember  = "params"
	jsonRPCResultMember  = "result"
	jsonRPCErrorMember   = "error"
)

type jsonObjectMembers map[string]struct{}

func (members jsonObjectMembers) add(member string) {
	members[member] = struct{}{}
}

func (members jsonObjectMembers) has(member string) bool {
	_, present := members[member]
	return present
}

func (members jsonObjectMembers) validateEnvelope() error {
	hasMethod := members.has(jsonRPCMethodMember)
	hasResult := members.has(jsonRPCResultMember)
	hasError := members.has(jsonRPCErrorMember)

	if hasMethod {
		return members.rejectUnknown(
			"request",
			jsonRPCVersionMember,
			jsonRPCIDMember,
			jsonRPCMethodMember,
			jsonRPCParamsMember,
		)
	}
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

func (members jsonObjectMembers) rejectUnknown(envelopeKind string, allowed ...string) error {
	allowedMembers := make(map[string]struct{}, len(allowed))
	for _, member := range allowed {
		allowedMembers[member] = struct{}{}
	}
	for member := range members {
		if _, ok := allowedMembers[member]; !ok {
			return fmt.Errorf("unknown JSON-RPC %s member %q", envelopeKind, member)
		}
	}
	return nil
}

func validateSingleUnambiguousValue(encoded []byte) error {
	decoder := jsontext.NewDecoder(bytes.NewReader(encoded))
	if _, err := decoder.ReadValue(); err != nil {
		return err
	}
	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		return errors.New("JSON-RPC message carries more than one JSON value")
	}
	return nil
}
