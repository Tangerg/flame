// Package transport owns the JSON-RPC 2.0 envelope vocabulary and encoding
// shared by the Runtime's streamable-HTTP binding and method dispatcher.
//
// Wire envelope types and encode/decode are re-exported from the MCP
// Go SDK's `jsonrpc` package — same vendor we use for our MCP
// integration, conformant JSON-RPC 2.0 implementation, "for use by
// mcp transport authors" per its own doc.
package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// Message is one JSON-RPC 2.0 envelope. Concrete types are
// [*Request] and [*Response]; type-switch to discriminate.
//
//   - Request with ID  → a Call
//   - Request no ID    → a Notification
//   - Response         → a Reply (Result XOR Error)
type Message = jsonrpc.Message

// Request is a Call (when ID is valid) or a Notification (when ID
// is zero). Use [Request.IsCall] to discriminate.
type Request = jsonrpc.Request

// Response is the reply to a Call. Either Result is set, or Error
// is set — never both.
type Response = jsonrpc.Response

// ID is an opaque JSON-RPC id. Flame narrows calls and replies to
// string ids only; DecodeMessage enforces that wire constraint before the SDK
// can coerce a numeric id.
type ID = jsonrpc.ID

// Error is the JSON-RPC error envelope. The wire shape carries
// Code (int64), Message (string), Data (raw JSON — typically
// [ProblemData]).
type Error = jsonrpc.Error

// EncodeMessage serializes a Message to wire bytes (no trailing
// newline). Delegates to the SDK.
func EncodeMessage(message Message) ([]byte, error) { return jsonrpc.EncodeMessage(message) }

// DecodeMessage parses wire bytes into either [*Request] or [*Response]. The
// SDK owns the JSON-RPC envelope semantics; this transport boundary first
// rejects duplicate JSON members so one byte sequence cannot be interpreted as
// two different requests by intermediaries that choose first-wins versus
// last-wins decoding.
func DecodeMessage(encoded []byte) (Message, error) {
	if err := validateUniqueJSONMembers(encoded); err != nil {
		return nil, err
	}
	message, err := jsonrpc.DecodeMessage(encoded)
	if err != nil {
		return nil, err
	}
	if request, ok := message.(*Request); ok && request.Method == "" {
		return nil, errors.New("JSON-RPC request method is empty")
	}
	return message, nil
}

func validateUniqueJSONMembers(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	parser := uniqueJSONParser{decoder: decoder}
	if _, err := parser.value(true); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON message contains more than one value")
		}
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

type uniqueJSONParser struct {
	decoder *json.Decoder
}

func (parser uniqueJSONParser) value(envelope bool) (jsonValueKind, error) {
	token, err := parser.decoder.Token()
	if err != nil {
		return jsonNull, err
	}
	if token == nil {
		return jsonNull, nil
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return jsonScalarKind(token)
	}
	switch delimiter {
	case '{':
		return parser.object(envelope)
	case '[':
		return parser.array()
	default:
		return jsonNull, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func jsonScalarKind(token json.Token) (jsonValueKind, error) {
	switch token.(type) {
	case string:
		return jsonString, nil
	case json.Number:
		return jsonNumber, nil
	case bool:
		return jsonBoolean, nil
	default:
		return jsonNull, fmt.Errorf("unexpected JSON scalar %T", token)
	}
}

func (parser uniqueJSONParser) object(envelope bool) (jsonValueKind, error) {
	members := make(jsonObjectMembers)
	for parser.decoder.More() {
		member, err := parser.memberName()
		if err != nil {
			return jsonObject, err
		}
		if err := members.add(member); err != nil {
			return jsonObject, err
		}
		valueKind, err := parser.value(false)
		if err != nil {
			return jsonObject, err
		}
		if envelope && member == jsonRPCIDMember && valueKind != jsonString {
			return jsonObject, errors.New(
				"JSON-RPC id must be a string; omit id for a notification",
			)
		}
	}
	if err := parser.close(json.Delim('}'), "JSON object is not closed"); err != nil {
		return jsonObject, err
	}
	if envelope {
		if err := members.validateEnvelope(); err != nil {
			return jsonObject, err
		}
	}
	return jsonObject, nil
}

func (parser uniqueJSONParser) memberName() (string, error) {
	token, err := parser.decoder.Token()
	if err != nil {
		return "", err
	}
	member, ok := token.(string)
	if !ok {
		return "", errors.New("JSON object member name is not a string")
	}
	return member, nil
}

func (parser uniqueJSONParser) array() (jsonValueKind, error) {
	for parser.decoder.More() {
		if _, err := parser.value(false); err != nil {
			return jsonArray, err
		}
	}
	if err := parser.close(json.Delim(']'), "JSON array is not closed"); err != nil {
		return jsonArray, err
	}
	return jsonArray, nil
}

func (parser uniqueJSONParser) close(expected json.Delim, message string) error {
	closing, err := parser.decoder.Token()
	if err != nil {
		return err
	}
	if closing != expected {
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

func (members jsonObjectMembers) add(member string) error {
	if _, exists := members[member]; exists {
		return fmt.Errorf("duplicate JSON member %q", member)
	}
	members[member] = struct{}{}
	return nil
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
