package llm

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestModelHTTPClientBoundsProviderFrames(t *testing.T) {
	client := newModelHTTPClient()
	transport, ok := client.Transport.(modelResponseRoundTripper)
	if !ok {
		t.Fatalf("model HTTP transport = %T, want modelResponseRoundTripper", client.Transport)
	}
	transport.base = modelRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: abc\n\n")),
			Request:    request,
		}, nil
	})
	transport.maxFrameBytes = 8
	client.Transport = transport
	response, err := client.Get("https://model.example/chat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	if _, err := io.ReadAll(response.Body); !errors.Is(err, errModelResponseFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want errModelResponseFrameTooLarge", err)
	}
}

type modelRoundTripFunc func(*http.Request) (*http.Response, error)

func (f modelRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
