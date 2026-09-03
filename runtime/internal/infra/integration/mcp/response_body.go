package mcp

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxMCPResponseFrameBytes int64 = 64 << 20

var errMCPResponseFrameTooLarge = errors.New("mcp: response frame too large")

// boundMCPResponseBody keeps the SDK as the MCP protocol owner while applying
// Flame's finite admission policy before its JSON and SSE decoders allocate
// from a server-controlled response. A stream may carry arbitrarily many SSE
// events; the bound resets only at an event boundary.
func boundMCPResponseBody(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	response.Body = &boundedMCPResponseBody{
		body:        response.Body,
		limit:       maxMCPResponseFrameBytes,
		remaining:   maxMCPResponseFrameBytes,
		eventStream: err == nil && strings.EqualFold(mediaType, "text/event-stream"),
	}
}

type boundedMCPResponseBody struct {
	body        io.ReadCloser
	limit       int64
	remaining   int64
	eventStream bool
	lineHasData bool
	failed      bool
}

func (b *boundedMCPResponseBody) Read(destination []byte) (int, error) {
	if b.failed {
		return 0, errMCPResponseFrameTooLarge
	}
	if len(destination) == 0 {
		return b.body.Read(destination)
	}
	read, readErr := b.body.Read(destination)
	if !b.eventStream {
		if int64(read) > b.remaining {
			accepted := int(b.remaining)
			b.remaining = 0
			b.failed = true
			return accepted, errMCPResponseFrameTooLarge
		}
		b.remaining -= int64(read)
		return read, readErr
	}

	for index, value := range destination[:read] {
		b.remaining--
		if b.remaining < 0 {
			b.failed = true
			return index, errMCPResponseFrameTooLarge
		}
		switch value {
		case '\n':
			if !b.lineHasData {
				b.remaining = b.limit
			}
			b.lineHasData = false
		case '\r':
		default:
			b.lineHasData = true
		}
	}
	return read, readErr
}

func (b *boundedMCPResponseBody) Close() error {
	return b.body.Close()
}
