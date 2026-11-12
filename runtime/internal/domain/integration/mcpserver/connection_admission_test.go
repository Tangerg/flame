package mcpserver

import "testing"

func TestConnectionAdmissionRejectsPointerVariants(t *testing.T) {
	for _, test := range []struct {
		name       string
		connection Connection
	}{
		{"mutable_stdio", &StdioConnection{Command: "mcp-server"}},
		{"mutable_http", &HTTPConnection{URL: "https://mcp.example.test"}},
		{"nil_stdio", (*StdioConnection)(nil)},
		{"nil_http", (*HTTPConnection)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := (Server{Connection: test.connection}).ValidateConnection(); err == nil {
				t.Fatal("connection admission accepted a pointer variant")
			}
		})
	}
}
