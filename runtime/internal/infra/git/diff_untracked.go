package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

const readBufferBytes = 64 << 10

const maxUntrackedDiffPaths = 10_000

// untrackedPaths lists untracked files (status ??), optionally under relPath.
func untrackedPaths(ctx context.Context, dir, relPath string) ([]string, error) {
	scopePath, err := gitPathRelativeToWorkspace(dir, relPath)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z", "--", scopePath)
	if err != nil {
		return nil, err
	}
	var paths []string
	for path := range bytes.SplitSeq(out, []byte{0}) {
		if len(path) != 0 {
			if len(paths) >= maxUntrackedDiffPaths {
				return nil, fmt.Errorf("%w: more than %d untracked diff paths", ErrResultTooLarge, maxUntrackedDiffPaths)
			}
			paths = append(paths, string(path))
		}
	}
	return paths, nil
}

// untrackedFileStat streams one untracked regular file for its Git-facing line
// count and binary bit. A symbolic link is one link-target line; its referent is
// never read as workspace material.
func untrackedFileStat(ctx context.Context, dir, rel string) (lines int, binary bool, err error) {
	path := filepath.Join(dir, rel)
	info, err := os.Lstat(path)
	if err != nil {
		return 0, false, fmt.Errorf("git: inspect untracked file %q: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 1, false, nil
	}
	if !info.Mode().IsRegular() {
		return 0, true, nil
	}
	file, opened, err := filesystem.OpenFileExpected(path, info, 0)
	if err != nil {
		if errors.Is(err, filesystem.ErrNotRegular) {
			return 0, true, nil
		}
		return 0, false, fmt.Errorf("git: open untracked file %q: %w", rel, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	source := cancellation.Reader(ctx, file)
	buffer := make([]byte, readBufferBytes)
	var last byte
	nonempty := false
	for {
		count, readErr := source.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			return 0, false, fmt.Errorf("git: read untracked file %q: %w", rel, readErr)
		}
		if count > 0 {
			nonempty = true
			chunk := buffer[:count]
			if bytes.IndexByte(chunk, 0) >= 0 {
				binary = true
				break
			}
			lines += bytes.Count(chunk, []byte{'\n'})
			last = chunk[len(chunk)-1]
		}
		if readErr == io.EOF {
			break
		}
	}
	if err := filesystem.VerifyPathVersion(file, opened, path); err != nil {
		return 0, false, fmt.Errorf("git: verify untracked file %q after reading: %w", rel, err)
	}
	if binary {
		return 0, true, nil
	}
	if nonempty && last != '\n' {
		lines++
	}
	return lines, false, nil
}
