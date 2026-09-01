package mcpserver

import (
	"strings"
	"testing"
)

func TestServerValidateRejectsUnsafeProcessConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Server)
		want   string
	}{
		{name: "command NUL", mutate: func(server *Server) { server.Command = "mcp\x00server" }, want: "command"},
		{name: "argument NUL", mutate: func(server *Server) { server.Args = []string{"ok", "bad\x00arg"} }, want: "args[1]"},
		{name: "directory NUL", mutate: func(server *Server) { server.Dir = "/tmp\x00escape" }, want: "dir"},
		{name: "empty environment key", mutate: func(server *Server) { server.Env = map[string]string{"": "value"} }, want: "env key"},
		{name: "environment key separator", mutate: func(server *Server) { server.Env = map[string]string{"A=B": "value"} }, want: "env key"},
		{name: "environment value NUL", mutate: func(server *Server) { server.Env = map[string]string{"TOKEN": "bad\x00value"} }, want: "TOKEN"},
		{name: "linker injection", mutate: func(server *Server) { server.Env = map[string]string{"ld_preload": "/tmp/evil.so"} }, want: "ld_preload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := Server{Name: testMCPServerName("local"), Transport: TransportStdio, Command: "mcp-server"}
			test.mutate(&server)
			err := server.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate err = %v, want detail %q", err, test.want)
			}
		})
	}
}
