package scopewire

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
	"github.com/Tangerg/scope/core/tool"
)

type Request chat.Request

func (Request) JSONSchemaModel() any           { return requestProjection{} }
func (r Request) MarshalJSON() ([]byte, error) { return chat.Request(r).MarshalJSON() }
func (r *Request) UnmarshalJSON(data []byte) error {
	var projection requestProjection
	if err := value.DecodeValue(data, &projection, "chat request"); err != nil {
		return err
	}
	if err := projection.Options.validateNumbers(); err != nil {
		return err
	}
	var native chat.Request
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*r = Request(native)
	return nil
}

type Message chat.Message

func (Message) JSONSchemaModel() any           { return messageProjection{} }
func (m Message) MarshalJSON() ([]byte, error) { return chat.Message(m).MarshalJSON() }
func (m *Message) UnmarshalJSON(data []byte) error {
	var projection messageProjection
	if err := value.DecodeValue(data, &projection, "chat message"); err != nil {
		return err
	}
	var native chat.Message
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*m = Message(native)
	return nil
}

type Response chat.Response

func (Response) JSONSchemaModel() any           { return responseProjection{} }
func (r Response) MarshalJSON() ([]byte, error) { return chat.Response(r).MarshalJSON() }
func (r *Response) UnmarshalJSON(data []byte) error {
	var projection responseProjection
	if err := value.DecodeValue(data, &projection, "chat response"); err != nil {
		return err
	}
	var native chat.Response
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*r = Response(native)
	return nil
}

type ResponseDelta chat.ResponseDelta

func (ResponseDelta) JSONSchemaModel() any           { return responseDeltaProjection{} }
func (r ResponseDelta) MarshalJSON() ([]byte, error) { return chat.ResponseDelta(r).MarshalJSON() }
func (r *ResponseDelta) UnmarshalJSON(data []byte) error {
	var projection responseDeltaProjection
	if err := value.DecodeValue(data, &projection, "chat response delta"); err != nil {
		return err
	}
	var native chat.ResponseDelta
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*r = ResponseDelta(native)
	return nil
}

type ToolOutput tool.Output

func (ToolOutput) JSONSchemaModel() any           { return toolOutputProjection{} }
func (o ToolOutput) MarshalJSON() ([]byte, error) { return tool.Output(o).MarshalJSON() }
func (o *ToolOutput) UnmarshalJSON(data []byte) error {
	var projection toolOutputProjection
	if err := value.DecodeValue(data, &projection, "tool output"); err != nil {
		return err
	}
	var native tool.Output
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*o = ToolOutput(native)
	return nil
}

type Block content.Block

func (Block) JSONSchemaModel() any           { return blockProjection{} }
func (b Block) MarshalJSON() ([]byte, error) { return content.Block(b).MarshalJSON() }
func (b *Block) UnmarshalJSON(data []byte) error {
	var projection blockProjection
	if err := value.DecodeValue(data, &projection, "content block"); err != nil {
		return err
	}
	var native content.Block
	if err := native.UnmarshalJSON(data); err != nil {
		return err
	}
	*b = Block(native)
	return nil
}

func (o optionsProjection) validateNumbers() error {
	for _, option := range []struct {
		name  string
		value *value.Number
	}{
		{"frequency_penalty", o.FrequencyPenalty},
		{"presence_penalty", o.PresencePenalty},
		{"temperature", o.Temperature},
		{"top_p", o.TopP},
	} {
		if option.value == nil {
			continue
		}
		if err := preserveFloat64(*option.value); err != nil {
			return fmt.Errorf("chat options %s: %w", option.name, err)
		}
	}
	return nil
}

// Scope's model options use float64, while portable JSON owns exact decimals.
// Compare decimal values after the native codec's shortest round trip so 0.7
// remains valid but an authored value cannot silently become another setting.
func preserveFloat64(number value.Number) error {
	native, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return err
	}
	if native == 0 {
		coefficient := number.String()
		if index := strings.IndexAny(coefficient, "eE"); index >= 0 {
			coefficient = coefficient[:index]
		}
		// Underflow must be refused before expanding an arbitrary exponent.
		if strings.Trim(coefficient, "-0.") != "" {
			return fmt.Errorf("%s cannot be represented by the model option codec", number.String())
		}
		return nil
	}
	before, ok := new(big.Rat).SetString(number.String())
	if !ok {
		return fmt.Errorf("invalid decimal %q", number.String())
	}
	after, ok := new(big.Rat).SetString(strconv.FormatFloat(native, 'g', -1, 64))
	if !ok || before.Cmp(after) != 0 {
		return fmt.Errorf("%s cannot be represented by the model option codec", number.String())
	}
	return nil
}
