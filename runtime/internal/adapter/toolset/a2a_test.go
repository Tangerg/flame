package toolset

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestA2AResponseRoundTripperBoundsServerFrames(t *testing.T) {
	client := newA2AHTTPClient()
	transport, ok := client.Transport.(a2aResponseRoundTripper)
	if !ok {
		t.Fatalf("A2A HTTP transport = %T, want a2aResponseRoundTripper", client.Transport)
	}
	transport.base = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 9))),
			Request:    request,
		}, nil
	})
	transport.maxFrameBytes = 8
	client.Transport = transport
	response, err := client.Get("https://agent.example/card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	if _, err := io.ReadAll(response.Body); !errors.Is(err, errA2AResponseFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want errA2AResponseFrameTooLarge", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
