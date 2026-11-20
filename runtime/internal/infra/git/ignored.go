package git

import (
	"bytes"
	"context"
	"strconv"
)

// IgnoredPaths asks Git about this directory page, without enumerating descendants.
// Git's index still exempts tracked paths from ignore rules.
func IgnoredPaths(ctx context.Context, root string, paths []string) (map[string]bool, error) {
	repository, err := IsRepo(ctx, root)
	if err != nil {
		return nil, err
	}
	if !repository {
		return nil, ErrNotRepo
	}
	ignored := make(map[string]bool)
	// Batches keep each check-ignore argument list well under the OS limit.
	const batch = 128
	for start := 0; start < len(paths); start += batch {
		args := append([]string{"check-ignore", "--"}, paths[start:min(start+batch, len(paths))]...)
		output, err := runAllowingExitCode(ctx, root, 1, args...)
		if err != nil {
			return nil, err
		}
		for value := range bytes.SplitSeq(bytes.TrimSuffix(output, []byte{10}), []byte{10}) {
			if len(value) > 0 {
				name := string(value)
				if name[0] == '"' {
					decoded, err := strconv.Unquote(name)
					if err != nil {
						return nil, err
					}
					name = decoded
				}
				ignored[name] = true
			}
		}
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
