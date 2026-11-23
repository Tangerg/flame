package httpresponse

import (
	"io"
	"mime"
	"net/http"
	"strings"
)

type BodyMode uint8

const (
	Document BodyMode = iota
	DocumentOrEvents
)

func LimitBody(response *http.Response, mode BodyMode, maxFrameBytes int64, tooLarge error) {
	validateFrameLimit(maxFrameBytes, tooLarge)
	if response == nil || response.Body == nil {
		return
	}
	var frames frameAdmission = &documentFrame{remaining: maxFrameBytes}
	if mode == DocumentOrEvents && isEventStream(response) {
		frames = &eventFrames{limit: maxFrameBytes, remaining: maxFrameBytes}
	}
	response.Body = &boundedBody{body: response.Body, frames: frames, tooLarge: tooLarge}
}

func isEventStream(response *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && err == nil && strings.EqualFold(mediaType, "text/event-stream")
}

func validateFrameLimit(maxFrameBytes int64, tooLarge error) {
	if maxFrameBytes <= 0 || tooLarge == nil {
		panic("httpresponse: a positive frame limit and size error are required")
	}
}

type frameAdmission interface {
	admit(read []byte) (accepted int, ok bool)
}

type boundedBody struct {
	body     io.ReadCloser
	frames   frameAdmission
	tooLarge error
	failed   bool
}

func (b *boundedBody) Read(destination []byte) (int, error) {
	if b.failed {
		return 0, b.tooLarge
	}
	read, readErr := b.body.Read(destination)
	if accepted, ok := b.frames.admit(destination[:read]); !ok {
		b.failed = true
		return accepted, b.tooLarge
	}
	return read, readErr
}

func (b *boundedBody) Close() error {
	return b.body.Close()
}

type documentFrame struct{ remaining int64 }

func (d *documentFrame) admit(read []byte) (int, bool) {
	if int64(len(read)) > d.remaining {
		accepted := int(d.remaining)
		d.remaining = 0
		return accepted, false
	}
	d.remaining -= int64(len(read))
	return len(read), true
}

type eventScan uint8

const (
	lineStart eventScan = iota
	lineData
	dataLineCR
	blankLineCR
)

type eventFrames struct {
	limit     int64
	remaining int64
	scan      eventScan
}

func (e *eventFrames) admit(read []byte) (int, bool) {
	for index, value := range read {
		if !e.admitByte(value) {
			return index, false
		}
	}
	return len(read), true
}

func (e *eventFrames) admitByte(value byte) bool {
	if e.scan == dataLineCR || e.scan == blankLineCR {
		// A split CRLF charges both bytes to the event the blank line ends.
		eventEnded := e.scan == blankLineCR
		e.scan = lineStart
		if value == '\n' {
			admitted := e.charge()
			if admitted && eventEnded {
				e.remaining = e.limit
			}
			return admitted
		}
		if eventEnded {
			e.remaining = e.limit
		}
	}
	if !e.charge() {
		return false
	}
	switch value {
	case '\r':
		if e.scan == lineData {
			e.scan = dataLineCR
		} else {
			e.scan = blankLineCR
		}
	case '\n':
		if e.scan != lineData {
			e.remaining = e.limit
		}
		e.scan = lineStart
	default:
		e.scan = lineData
	}
	return true
}

func (e *eventFrames) charge() bool {
	e.remaining--
	return e.remaining >= 0
}
