package process

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

const (
	indexFile         = "index"
	sharedIndexPrefix = "sharedindex."
	infoDirectory     = "info"
	objectsDirectory  = "objects"
)

const unbornHead = "ref: refs/heads/unborn\n"

type repositoryQuery struct {
	ctx         context.Context
	env, prefix []string
}

type WorktreeQuery func(input io.Reader, args ...string) (Result, error)

func (q repositoryQuery) run(input io.Reader, args ...string) (Result, error) {
	return Run(q.ctx, q.env, input, append(slices.Clone(q.prefix), args...)...)
}

func (q repositoryQuery) path(args ...string) (string, error) {
	r, err := q.run(nil, append([]string{"rev-parse", "--path-format=absolute"}, args...)...)
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return "", fmt.Errorf("git: resolve repository metadata: %s", r.Stderr)
	}
	return strings.TrimSuffix(string(r.Stdout), "\n"), nil
}

func CheckIgnore(ctx context.Context, dir string, names io.Reader) (Result, error) {
	return ObserveWorktree(ctx, dir, func(query WorktreeQuery) (Result, error) {
		return query(names, "check-ignore", "--stdin", "-z")
	})
}

// ObserveWorktree lets related queries share a private index and object store.
func ObserveWorktree(ctx context.Context, dir string, inspect func(WorktreeQuery) (Result, error)) (_ Result, err error) {
	prefix, env := worktreeInvocation(dir)
	q := repositoryQuery{ctx: ctx, env: env, prefix: prefix}
	root, err := q.run(nil, "rev-parse", "--show-toplevel")
	if err != nil || root.ExitCode != 0 {
		return root, err
	}
	tree := strings.TrimSuffix(string(root.Stdout), "\n")
	view, err := newReadDirectory(tree)
	if err != nil {
		return Result{}, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(view)) }()
	common, err := q.path("--git-common-dir")
	if err != nil {
		return Result{}, err
	}
	env, err = q.installReadConfig(view, readViewEnvironment(env, view))
	if err != nil {
		return Result{}, err
	}
	if err := q.installReadHead(view); err != nil {
		return Result{}, err
	}
	if err := q.installWorktreeMetadata(view, common); err != nil {
		return Result{}, err
	}
	objects := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(filepath.Join(common, objectsDirectory))
	q.env = append(env, "GIT_WORK_TREE="+tree, `GIT_ALTERNATE_OBJECT_DIRECTORIES="`+objects+`"`)
	return inspect(q.run)
}

func readViewEnvironment(env []string, view string) []string {
	return append(slices.Clone(env), "GIT_DIR="+view, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
}

func (q repositoryQuery) installWorktreeMetadata(view, common string) error {
	gitDir, err := q.path("--git-dir")
	if err != nil {
		return err
	}
	if err := copyReadMaterial(filepath.Join(gitDir, indexFile), filepath.Join(view, indexFile)); err != nil {
		return err
	}
	// Git refreshes a shared index's mtime even when only identifying it.
	// It must choose the companion after the entire index family is private.
	entries, err := os.ReadDir(gitDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !sharedIndexName(name) {
			continue
		}
		if err := copyReadMaterial(filepath.Join(gitDir, name), filepath.Join(view, name)); err != nil {
			return err
		}
	}
	for _, name := range []string{"attributes", "exclude"} {
		if err := copyReadMaterial(filepath.Join(common, infoDirectory, name), filepath.Join(view, infoDirectory, name)); err != nil {
			return err
		}
	}
	return nil
}

func sharedIndexName(name string) bool {
	digest, ok := strings.CutPrefix(name, sharedIndexPrefix)
	if !ok || len(digest) != hex.EncodedLen(sha1.Size) && len(digest) != hex.EncodedLen(sha256.Size) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func newReadDirectory(tree string) (string, error) {
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(tree, parent); err == nil && filepath.IsLocal(rel) {
		return "", errors.New("git: inspection requires a temporary directory outside the worktree")
	}
	view, err := os.MkdirTemp(parent, "flame-git-read-*")
	if err != nil {
		return "", err
	}
	for _, dir := range []string{objectsDirectory, "refs", infoDirectory} {
		if err := os.Mkdir(filepath.Join(view, dir), filesystem.PrivateDirectoryMode); err != nil {
			return "", errors.Join(err, os.RemoveAll(view))
		}
	}
	return view, nil
}

func (q repositoryQuery) installReadHead(view string) error {
	r, err := q.run(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return err
	}
	var head []byte
	switch r.ExitCode {
	case 0:
		head = r.Stdout
	case 1:
		head = []byte(unbornHead)
	default:
		return fmt.Errorf("git: resolve comparison HEAD: %s", r.Stderr)
	}
	return os.WriteFile(filepath.Join(view, "HEAD"), head, filesystem.PrivateFileMode)
}

// Only display and index interpretation settings cross into the comparison
// repository. A config rewrite between discovery and execution cannot add a
// command, hook, remote or include to this closed list.
func (q repositoryQuery) installReadConfig(view string, env []string) ([]string, error) {
	r, err := q.run(nil, "config", "--null", "--get-regexp", `^(core\.(filemode|ignorecase|symlinks|autocrlf|eol|safecrlf|trustctime|checkstat|precomposeunicode|excludesfile|attributesfile)|extensions\.objectformat)$`)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 && r.ExitCode != 1 {
		return nil, fmt.Errorf("git: read comparison settings: %s", r.Stderr)
	}
	config := "[core]\nrepositoryformatversion = 0\n"
	var count int
	for entry := range bytes.SplitSeq(r.Stdout, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		key, value, ok := strings.Cut(string(entry), "\n")
		if !ok {
			return nil, errors.New("git: invalid comparison setting")
		}
		if key == "extensions.objectformat" {
			switch value {
			case "sha1":
			case "sha256":
				config = "[core]\nrepositoryformatversion = 1\n[extensions]\nobjectformat = sha256\n"
			default:
				return nil, fmt.Errorf("git: unsupported object format %q", value)
			}
			continue
		}
		n := strconv.Itoa(count)
		env = append(env, "GIT_CONFIG_KEY_"+n+"="+key, "GIT_CONFIG_VALUE_"+n+"="+value)
		count++
	}
	if err := os.WriteFile(filepath.Join(view, "config"), []byte(config), filesystem.PrivateFileMode); err != nil {
		return nil, err
	}
	return append(env, "GIT_CONFIG_COUNT="+strconv.Itoa(count)), nil
}

func copyReadMaterial(source, destination string) (err error) {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("git: expected regular metadata at %q", source)
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filesystem.PrivateFileMode)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
