package mcpserver

import (
	"strings"
	"testing"
)

func TestServerValidateRejectsAmbiguousOrInvalidHTTPHeaders(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*HTTPConnection)
		want   string
	}{
		{name: "blank authorization", mutate: func(server *HTTPConnection) { server.Authorization = " \t" }, want: "authorization"},
		{name: "authorization leading whitespace", mutate: func(server *HTTPConnection) { server.Authorization = " Bearer secret" }, want: "authorization"},
		{name: "authorization trailing whitespace", mutate: func(server *HTTPConnection) { server.Authorization = "Bearer secret " }, want: "authorization"},
		{name: "authorization newline", mutate: func(server *HTTPConnection) { server.Authorization = "Bearer secret\r\nInjected: yes" }, want: "authorization"},
		{name: "invalid header name", mutate: func(server *HTTPConnection) { server.Headers = map[string]string{"Bad Header": "value"} }, want: "Bad Header"},
		{name: "invalid header value", mutate: func(server *HTTPConnection) { server.Headers = map[string]string{"X-Key": "value\nInjected"} }, want: "X-Key"},
		{name: "authorization duplicate", mutate: func(server *HTTPConnection) { server.Headers = map[string]string{"authorization": "Bearer secret"} }, want: "duplicate authorization"},
		{name: "case-insensitive duplicate", mutate: func(server *HTTPConnection) { server.Headers = map[string]string{"X-Key": "one", "x-key": "two"} }, want: "differ only by case"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection := HTTPConnection{URL: "https://example.com/mcp"}
			test.mutate(&connection)
			server := Server{Source: UserSource(), Name: testMCPServerName("remote"), Connection: connection}
			err := server.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate err = %v, want detail %q", err, test.want)
			}
		})
	}
}
