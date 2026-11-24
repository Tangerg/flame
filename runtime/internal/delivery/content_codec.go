package delivery

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type contentDecodeError struct {
	field  string
	detail string
}

func decodeContent(encoded protocol.ContentBlock) (transcript.ContentBlock, *contentDecodeError) {
	block := transcript.ContentBlock{Text: encoded.Text, Label: encoded.Label}
	switch encoded.Type {
	case protocol.ContentBlockText:
		block.Kind = transcript.TextContent
	case protocol.ContentBlockImage:
		block.Kind = transcript.ImageContent
	case protocol.ContentBlockReference:
		block.Kind = transcript.ReferenceContent
	default:
		return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentKindField), detail: "must be text, image or reference"}
	}
	if encoded.Mime != "" {
		mediaType, _, err := mime.ParseMediaType(encoded.Mime)
		if err != nil {
			return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentMediaTypeField), detail: fmt.Sprintf("must be a valid MIME type, got %q", encoded.Mime)}
		}
		block.MediaType = mediaType
	}
	if encoded.Data != "" {
		data, err := base64.StdEncoding.DecodeString(encoded.Data)
		if err != nil {
			return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentBytesField), detail: "must be valid base64"}
		}
		block.Bytes = data
	}
	if err := block.Validate(); err != nil {
		invalid, ok := errors.AsType[*transcript.ContentError](err)
		if !ok {
			return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentKindField), detail: err.Error()}
		}
		return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(invalid.Field), detail: invalid.Detail}
	}
	return block, nil
}

func wireContentField(field transcript.ContentField) string {
	switch field {
	case transcript.ContentTextField:
		return "text"
	case transcript.ContentMediaTypeField:
		return "mime"
	case transcript.ContentBytesField:
		return "data"
	case transcript.ContentLabelField:
		return "label"
	default:
		return "type"
	}
}

func encodeContent(block transcript.ContentBlock) (protocol.ContentBlock, error) {
	switch block.Kind {
	case transcript.TextContent:
		return protocol.ContentBlock{Type: protocol.ContentBlockText, Text: block.Text}, nil
	case transcript.ImageContent:
		return protocol.ContentBlock{
			Type: protocol.ContentBlockImage,
			Mime: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	case transcript.ReferenceContent:
		return protocol.ContentBlock{Type: protocol.ContentBlockReference, Label: block.Label, Text: block.Text}, nil
	default:
		return protocol.ContentBlock{}, fmt.Errorf("unknown content kind %q", block.Kind)
	}
}
