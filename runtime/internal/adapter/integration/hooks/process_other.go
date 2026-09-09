//go:build !unix && !windows

package hooks

import (
	"context"
	"os/exec"
)

func hookShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
