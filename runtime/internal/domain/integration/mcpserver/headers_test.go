package mcpserver

import (
	"strings"
	"testing"
)

func TestServerValidateRejectsAmbiguousOrInvalidHTTPHeaders(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Server)
		want   string
	}{
		{name: "blank authorization", mutate: func(server *Server) { server.Authorization = " \t" }, want: "authorization"},
		{name: "authorization leading whitespace", mutate: func(server *Server) { server.Authorization = " Bearer secret" }, want: "authorization"},
		{name: "authorization trailing whitespace", mutate: func(server *Server) { server.Authorization = "Bearer secret " }, want: "authorization"},
		{name: "authorization newline", mutate: func(server *Server) { server.Authorization = "Bearer secret\r\nInjected: yes" }, want: "authorization"},
		{name: "invalid header name", mutate: func(server *Server) { server.Headers = map[string]string{"Bad Header": "value"} }, want: "Bad Header"},
		{name: "invalid header value", mutate: func(server *Server) { server.Headers = map[string]string{"X-Key": "value\nInjected"} }, want: "X-Key"},
		{name: "authorization duplicate", mutate: func(server *Server) { server.Headers = map[string]string{"authorization": "Bearer secret"} }, want: "duplicate authorization"},
		{name: "case-insensitive duplicate", mutate: func(server *Server) { server.Headers = map[string]string{"X-Key": "one", "x-key": "two"} }, want: "differ only by case"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := Server{
				Name: testMCPServerName("remote"), Transport: TransportStreamableHTTP, URL: "https://example.com/mcp",
			}
			test.mutate(&server)
			err := server.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate err = %v, want detail %q", err, test.want)
			}
		})
	}
}
