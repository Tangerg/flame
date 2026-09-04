package mcpserver

import (
	"errors"
	"testing"
)

func TestConnectionStatusValidatesCompleteProjection(t *testing.T) {
	name := testMCPServerName("files")
	for _, status := range []ConnectionStatus{
		{Name: name, State: ConnectionConnecting},
		{Name: name, State: ConnectionConnected, ToolCount: MaxRemoteToolsPerServer},
		{Name: name, State: ConnectionFailed},
		{Name: name, State: ConnectionNeedsAuth},
	} {
		if err := status.Validate(); err != nil {
			t.Errorf("valid status %+v rejected: %v", status, err)
		}
	}

	for caseName, status := range map[string]ConnectionStatus{
		"missing server":         {State: ConnectionConnected},
		"unknown state":          {Name: name, State: "ready"},
		"negative count":         {Name: name, State: ConnectionConnected, ToolCount: -1},
		"oversized count":        {Name: name, State: ConnectionConnected, ToolCount: MaxRemoteToolsPerServer + 1},
		"count while connecting": {Name: name, State: ConnectionConnecting, ToolCount: 1},
		"count while failed":     {Name: name, State: ConnectionFailed, ToolCount: 1},
	} {
		t.Run(caseName, func(t *testing.T) {
			if err := status.Validate(); !errors.Is(err, ErrInvalidConnectionStatus) {
				t.Fatalf("Validate error = %v, want ErrInvalidConnectionStatus", err)
			}
		})
	}
}
