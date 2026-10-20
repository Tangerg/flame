package httpresponse

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

var errTestFrameTooLarge = errors.New("test frame too large")

func TestLimitBodyAcceptsExactDocument(t *testing.T) {
	response := responseWithBody("application/json", strings.Repeat("x", 8))
	LimitBody(response, 8, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
}

func TestLimitBodyRejectsOversizedDocumentWithoutDraining(t *testing.T) {
	response := responseWithBody("application/json", strings.Repeat("x", 9))
	LimitBody(response, 8, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want size error", err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
	if read, err := response.Body.Read(make([]byte, 1)); read != 0 || !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("read after rejection = (%d, %v), want stable size error", read, err)
	}
}

func TestLimitBodyResetsAtSSEEventBoundaries(t *testing.T) {
	response := responseWithBody("text/event-stream; charset=utf-8", "data: a\r\n\r\ndata: b\r\n\r\n")
	LimitBody(response, 11, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "data: a\r\n\r\ndata: b\r\n\r\n" {
		t.Fatalf("SSE content = %q", content)
	}
}

func TestLimitBodyRejectsOversizedSSEEvent(t *testing.T) {
	response := responseWithBody("text/event-stream", "data: a\ndata: b\n\n")
	LimitBody(response, 10, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want size error", err)
	}
	if string(content) != "data: a\nda" {
		t.Fatalf("accepted SSE prefix = %q, want bounded prefix", content)
	}
}

func responseWithBody(contentType, content string) *http.Response {
	return &http.Response{
		Header: http.Header{"Content-Type": {contentType}},
		Body:   io.NopCloser(strings.NewReader(content)),
	}
}
