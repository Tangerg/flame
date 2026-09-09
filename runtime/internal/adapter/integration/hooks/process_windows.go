//go:build windows

package hooks

import (
	"context"
	"os/exec"
)

func hookShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
}
