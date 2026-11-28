package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

const (
	workingDirectoryPrefix = "./"
	noIgnoredPaths         = 1
)

func IgnoredChildren(ctx context.Context, dir string, names []string) (map[string]bool, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !repository {
		return nil, ErrNotRepo
	}
	var input strings.Builder
	// A leading colon is pathspec syntax, and check-ignore rejects literal pathspecs.
	for _, name := range names {
		input.WriteString(workingDirectoryPrefix)
		input.WriteString(name)
		input.WriteByte(0)
	}
	result, err := process.CheckIgnore(ctx, dir, strings.NewReader(input.String()))
	output, err := readCommandOutput([]string{"check-ignore"}, result, err, noIgnoredPaths)
	if err != nil {
		return nil, err
	}
	ignored := make(map[string]bool)
	for value := range bytes.SplitSeq(output, []byte{0}) {
		if len(value) == 0 {
			continue
		}
		name, ok := strings.CutPrefix(string(value), workingDirectoryPrefix)
		if !ok {
			return nil, fmt.Errorf("git check-ignore: unqualified path %q", value)
		}
		ignored[name] = true
	}
	return ignored, nil
}

func HasTrackedFiles(ctx context.Context, root, selected string) (bool, error) {
	scope, err := workspacePathspec(root, selected)
	if err != nil {
		return false, err
	}
	output, err := run(ctx, root, "ls-files", "--cached", "-z", "--", scope)
	return len(output) > 0, err
}
