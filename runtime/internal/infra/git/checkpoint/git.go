package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

// A fixed identity and disabled signing keep commits independent of the user's
// global git config; path names stay literal so glob syntax cannot expand the
// admitted set.
func (s *Store) git(ctx context.Context, gitDir, workTree string, args ...string) error {
	_, err := s.gitOutput(ctx, gitDir, workTree, args...)
	return err
}

func (s *Store) gitOutput(ctx context.Context, gitDir, workTree string, args ...string) ([]byte, error) {
	environment := []string{
		"GIT_DIR=" + gitDir,
		"GIT_AUTHOR_NAME=flame", "GIT_AUTHOR_EMAIL=flame@localhost",
		"GIT_COMMITTER_NAME=flame", "GIT_COMMITTER_EMAIL=flame@localhost",
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_LITERAL_PATHSPECS=1",
		"LC_ALL=C", "LANG=C",
	}
	if workTree != "" {
		environment = append(environment, "GIT_WORK_TREE="+workTree)
	}
	result, err := process.Run(ctx, environment, args...)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: git %s: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return result.Stdout, &gitCommandError{
			operation: args[0], exitCode: result.ExitCode, diagnostic: strings.TrimSpace(result.Stderr),
		}
	}
	return result.Stdout, nil
}

type gitCommandError struct {
	operation  string
	exitCode   int
	diagnostic string
}

func (g *gitCommandError) Error() string {
	if g.diagnostic == "" {
		return fmt.Sprintf("checkpoint: git %s: exit code %d", g.operation, g.exitCode)
	}
	return fmt.Sprintf("checkpoint: git %s: %s: exit code %d", g.operation, g.diagnostic, g.exitCode)
}

func (g *gitCommandError) ExitCode() int { return g.exitCode }

func gitExitCode(err error) int {
	if exited, ok := errors.AsType[*gitCommandError](err); ok {
		return exited.ExitCode()
	}
	return -1
}

func gitIn(ctx context.Context, cwd string, args ...string) (string, error) {
	output, err := gitInOutput(ctx, cwd, args...)
	return strings.TrimSpace(string(output)), err
}

func gitInOutput(ctx context.Context, cwd string, args ...string) ([]byte, error) {
	result, err := process.At(ctx, cwd, args...)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: source git %s: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return nil, &gitCommandError{
			operation: args[0], exitCode: result.ExitCode, diagnostic: strings.TrimSpace(result.Stderr),
		}
	}
	return result.Stdout, nil
}
