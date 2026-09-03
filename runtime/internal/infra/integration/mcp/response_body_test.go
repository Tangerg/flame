package mcp

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBoundedMCPResponseBodyAcceptsExactJSONDocument(t *testing.T) {
	body := newBoundedMCPResponseBody(strings.Repeat("x", 8), false, 8)
	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
}

func TestBoundedMCPResponseBodyRejectsOversizedJSONDocument(t *testing.T) {
	body := newBoundedMCPResponseBody(strings.Repeat("x", 9), false, 8)
	content, err := io.ReadAll(body)
	if !errors.Is(err, errMCPResponseFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want errMCPResponseFrameTooLarge", err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
	if read, err := body.Read(make([]byte, 1)); read != 0 || !errors.Is(err, errMCPResponseFrameTooLarge) {
		t.Fatalf("read after rejection = (%d, %v), want stable size error", read, err)
	}
}

func TestBoundedMCPResponseBodyResetsAtSSEEventBoundaries(t *testing.T) {
	body := newBoundedMCPResponseBody("data: a\n\ndata: b\n\n", true, 9)
	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "data: a\n\ndata: b\n\n" {
		t.Fatalf("SSE content = %q", content)
	}
}

func TestBoundedMCPResponseBodyRejectsOversizedSSEEvent(t *testing.T) {
	body := newBoundedMCPResponseBody("data: abc\n\n", true, 8)
	content, err := io.ReadAll(body)
	if !errors.Is(err, errMCPResponseFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want errMCPResponseFrameTooLarge", err)
	}
	if string(content) != "data: ab" {
		t.Fatalf("accepted SSE prefix = %q, want bounded prefix", content)
	}
}

func TestBoundMCPResponseBodySelectsSSEFraming(t *testing.T) {
	client, err := endpointHTTPClient("https://example.com/mcp", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := client.Transport.(*headerRoundTripper)
	transport.base = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader("data: ok\r\n\r\n")),
			Request:    request,
		}, nil
	})
	response, err := client.Get("https://example.com/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	body, ok := response.Body.(*boundedMCPResponseBody)
	if !ok || !body.eventStream || body.remaining != maxMCPResponseFrameBytes {
		t.Fatalf("bounded response body = %#v", response.Body)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newBoundedMCPResponseBody(content string, eventStream bool, limit int64) *boundedMCPResponseBody {
	return &boundedMCPResponseBody{
		body:        io.NopCloser(strings.NewReader(content)),
		limit:       limit,
		remaining:   limit,
		eventStream: eventStream,
	}
}
