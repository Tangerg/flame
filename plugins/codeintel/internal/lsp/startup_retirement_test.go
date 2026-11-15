package lsp

import (
	"context"
	"errors"
	"testing"
)

func TestFailedStartupRetainsUnconfirmedRetirement(t *testing.T) {
	for _, test := range []struct {
		name       string
		cleanupErr error
	}{
		{name: "retired"},
		{name: "retirement unconfirmed", cleanupErr: errors.New("original process group remains owned")},
	} {
		t.Run(test.name, func(t *testing.T) {
			servers := mustNewServers(t, nil)
			defer func() { _ = servers.Close() }()
			original := inertClient(t, test.cleanupErr)
			replacement, _ := connectedTestClient(t, replyEmpty)
			initializationErr := errors.New("language server rejected initialization")
			launches := 0
			servers.launch = func(context.Context, ServerSpec, string) (*client, error) {
				launches++
				if launches == 1 {
					return original, initializationErr
				}
				return replacement, nil
			}
			root, spec := t.TempDir(), ServerSpec{Name: "test"}
			if c, err := servers.clientFor(t.Context(), root, spec); c != nil || !errors.Is(err, initializationErr) || test.cleanupErr != nil && !errors.Is(err, test.cleanupErr) {
				t.Fatalf("failed initialization = %p, %v", c, err)
			}
			c, err := servers.clientFor(t.Context(), root, spec)
			if test.cleanupErr == nil {
				if err != nil || c != replacement || launches != 2 {
					t.Fatalf("retired startup could not be replaced: client=%p, launches=%d, error=%v", c, launches, err)
				}
			} else if c != nil || !errors.Is(err, initializationErr) || !errors.Is(err, test.cleanupErr) || launches != 1 {
				t.Fatalf("replacement crossed unconfirmed retirement: client=%p, launches=%d, error=%v", c, launches, err)
			}
			if err := servers.Close(); !errors.Is(err, test.cleanupErr) {
				t.Fatalf("Close lost original retirement failure: %v", err)
			}
		})
	}
}
