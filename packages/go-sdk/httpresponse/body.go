// Package httpresponse applies finite document and event admission to HTTP
// response bodies before protocol SDKs decode server-controlled material.
package httpresponse

import (
	"io"
	"mime"
	"net/http"
	"strings"
)

// LimitBody bounds one non-streaming response document or one SSE event. SSE
// streams may carry arbitrarily many events; their budget resets only at a
// blank-line event boundary. The caller owns the protocol-specific limit and
// error vocabulary.
func LimitBody(response *http.Response, maxFrameBytes int64, tooLarge error) {
	validateFrameLimit(maxFrameBytes, tooLarge)
	if response == nil || response.Body == nil {
		return
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	response.Body = &boundedBody{
		body:        response.Body,
		limit:       maxFrameBytes,
		remaining:   maxFrameBytes,
		tooLarge:    tooLarge,
		eventStream: err == nil && strings.EqualFold(mediaType, "text/event-stream"),
	}
}

func validateFrameLimit(maxFrameBytes int64, tooLarge error) {
	if maxFrameBytes <= 0 || tooLarge == nil {
		panic("httpresponse: a positive frame limit and size error are required")
	}
}

type boundedBody struct {
	body        io.ReadCloser
	limit       int64
	remaining   int64
	tooLarge    error
	eventStream bool
	lineHasData bool
	failed      bool
}

func (b *boundedBody) Read(destination []byte) (int, error) {
	if b.failed {
		return 0, b.tooLarge
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
			return accepted, b.tooLarge
		}
		b.remaining -= int64(read)
		return read, readErr
	}

	for index, value := range destination[:read] {
		b.remaining--
		if b.remaining < 0 {
			b.failed = true
			return index, b.tooLarge
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

func (b *boundedBody) Close() error {
	return b.body.Close()
}
