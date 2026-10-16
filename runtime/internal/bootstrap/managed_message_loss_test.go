package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

const messageLossProcessEnv = "FLAME_MESSAGE_LOSS_TEST_PROCESS"

func runMessageLossFrameServer() {
	server := sdk.NewServer(&sdk.Implementation{Name: "frame-boundary", Version: "1"}, &sdk.ServerOptions{
		Instructions: os.Getenv("FLAME_MESSAGE_LOSS_TEST_INSTRUCTIONS"),
	})
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func TestProxyPreservesSDKAdmittedDiscoveryFrames(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "memory", "never-match", "response")
	large := strings.Repeat("x", 80<<10)
	for _, test := range []struct {
		name, clientName, instructions string
	}{
		{"request", large, "small response"},
		{"response", "small request", large},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, filepath.Join(source, "bin", "message-loss"), evidence, os.Args[0], "never-match", "", "response")
			command.Env = append(os.Environ(), messageLossProcessEnv+"=1", "FLAME_MESSAGE_LOSS_TEST_INSTRUCTIONS="+test.instructions)
			command.Stderr = os.Stderr
			client := sdk.NewClient(&sdk.Implementation{Name: test.clientName, Version: "1"}, nil)
			session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if actual := session.InitializeResult().Instructions; actual != test.instructions {
				t.Errorf("discovery instructions changed: bytes=%d, want %d", len(actual), len(test.instructions))
			}
			// The loss proxy hard-stops its backend on transport retirement. Join it
			// even when that deliberate stop wins the backend's normal EOF exit.
			if err := session.Close(); err != nil {
				exit, stopped := errors.AsType[*exec.ExitError](err)
				if !stopped || exit.ExitCode() != 1 {
					t.Errorf("loss proxy hard-stop cleanup: %v", err)
				}
			}
			if command.ProcessState == nil {
				t.Error("transport retirement did not join its proxy")
			}
			if ctx.Err() != nil {
				t.Errorf("discovery or retirement exceeded its bound: %v", ctx.Err())
			}
		})
	}
}
