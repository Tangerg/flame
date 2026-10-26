package httpresponse

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientBoundsResponseBeforeProtocolDecoding(t *testing.T) {
	tooLarge := errors.New("test client response too large")
	client := NewClient(8, tooLarge)
	transport, ok := client.Transport.(responseRoundTripper)
	if !ok {
		t.Fatalf("HTTP transport = %T, want responseRoundTripper", client.Transport)
	}
	transport.base = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 9))),
			Request:    request,
		}, nil
	})
	client.Transport = transport

	response, err := client.Get("https://provider.example/api")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	content, err := io.ReadAll(response.Body)
	if !errors.Is(err, tooLarge) {
		t.Fatalf("ReadAll error = %v, want configured size error", err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted response = %q, want eight bytes", content)
	}
}

func TestNewClientRejectsIncompleteAdmissionPolicy(t *testing.T) {
	cases := []struct {
		name     string
		limit    int64
		tooLarge error
	}{
		{name: "zero limit", tooLarge: errors.New("too large")},
		{name: "missing error", limit: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("NewClient accepted an incomplete admission policy")
				}
			}()
			NewClient(test.limit, test.tooLarge)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
