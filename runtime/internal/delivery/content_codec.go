package delivery

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type encodedContent struct {
	kind protocol.ContentBlockType
	text string
	mime string
	data string
}

type contentDecodeError struct {
	field  string
	detail string
}

// decodeContent decodes the wire encoding of one block; the content shape
// itself is the transcript's rule, addressed here by wire field name.
func decodeContent(encoded encodedContent) (transcript.ContentBlock, *contentDecodeError) {
	block := transcript.ContentBlock{Text: encoded.text}
	switch encoded.kind {
	case protocol.ContentBlockText:
		block.Kind = transcript.TextContent
	case protocol.ContentBlockImage:
		block.Kind = transcript.ImageContent
	default:
		return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentKindField), detail: "must be text or image"}
	}
	if encoded.mime != "" {
		mediaType, _, err := mime.ParseMediaType(encoded.mime)
		if err != nil {
			return transcript.ContentBlock{}, &contentDecodeError{field: wireContentField(transcript.ContentMediaTypeField), detail: fmt.Sprintf("must be a valid MIME type, got %q", encoded.mime)}
		}
		block.MediaType = mediaType
	}
	if encoded.data != "" {
		data, err := base64.StdEncoding.DecodeString(encoded.data)
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

// wireContentField names a content field as the protocol's ContentBlock does.
func wireContentField(field transcript.ContentField) string {
	switch field {
	case transcript.ContentTextField:
		return "text"
	case transcript.ContentMediaTypeField:
		return "mime"
	case transcript.ContentBytesField:
		return "data"
	default:
		return "type"
	}
}

func encodeContent(block transcript.ContentBlock) (encodedContent, error) {
	switch block.Kind {
	case transcript.TextContent:
		return encodedContent{kind: protocol.ContentBlockText, text: block.Text}, nil
	case transcript.ImageContent:
		return encodedContent{
			kind: protocol.ContentBlockImage,
			mime: block.MediaType,
			data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return encodedContent{}, fmt.Errorf("unknown content kind %q", block.Kind)
	}
}
