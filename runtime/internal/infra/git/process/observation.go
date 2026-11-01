package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type repositoryQuery struct {
	ctx         context.Context
	env, prefix []string
}

func (q repositoryQuery) run(args ...string) (Result, error) {
	return Run(q.ctx, q.env, append(slices.Clone(q.prefix), args...)...)
}
func (q repositoryQuery) path(args ...string) (string, error) {
	r, err := q.run(append([]string{"rev-parse", "--path-format=absolute"}, args...)...)
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return "", fmt.Errorf("git: resolve repository metadata: %s", r.Stderr)
	}
	return strings.TrimSuffix(string(r.Stdout), "\n"), nil
}

func observeWorktree(ctx context.Context, env, prefix, args []string) (_ Result, err error) {
	q := repositoryQuery{ctx: ctx, env: env, prefix: prefix}
	noIndex := len(args) > 1 && args[0] == "diff" && args[1] == "--no-index"
	tree := ""
	if !noIndex {
		root, err := q.run("rev-parse", "--show-toplevel")
		if err != nil || root.ExitCode != 0 {
			return root, err
		}
		tree = strings.TrimSuffix(string(root.Stdout), "\n")
	}
	view, err := newReadDirectory(tree)
	if err != nil {
		return Result{}, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(view)) }()
	baseEnv := append(slices.Clone(env), "GIT_DIR="+view, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
	if noIndex {
		if err := os.WriteFile(filepath.Join(view, "HEAD"), []byte("ref: refs/heads/unborn\n"), 0600); err != nil {
			return Result{}, err
		}
		return Run(ctx, baseEnv, append(slices.Clone(prefix), args...)...)
	}
	gitDir, err := q.path("--git-dir")
	if err != nil {
		return Result{}, err
	}
	common, err := q.path("--git-common-dir")
	if err != nil {
		return Result{}, err
	}

	env, err = q.installReadConfig(view, baseEnv)
	if err != nil {
		return Result{}, err
	}
	if err := q.installReadHead(view); err != nil {
		return Result{}, err
	}
	if err := copyReadMaterial(filepath.Join(gitDir, "index"), filepath.Join(view, "index")); err != nil {
		return Result{}, err
	}
	shared, err := q.path("--shared-index-path")
	if err != nil {
		return Result{}, err
	}
	if shared != "" {
		if err := copyReadMaterial(shared, filepath.Join(view, filepath.Base(shared))); err != nil {
			return Result{}, err
		}
	}
	for _, name := range []string{"attributes", "exclude"} {
		if err := copyReadMaterial(filepath.Join(common, "info", name), filepath.Join(view, "info", name)); err != nil {
			return Result{}, err
		}
	}
	env = append(env, "GIT_WORK_TREE="+tree, "GIT_OBJECT_DIRECTORY="+filepath.Join(common, "objects"))
	result, err := Run(ctx, env, append(slices.Clone(prefix), args...)...)
	return result, err
}

func newReadDirectory(tree string) (string, error) {
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", err
	}
	if tree != "" {
		rel, err := filepath.Rel(tree, parent)
		if err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return "", errors.New("git: inspection requires a temporary directory outside the worktree")
		}
	}
	view, err := os.MkdirTemp(parent, "flame-git-read-*")
	if err != nil {
		return "", err
	}
	for _, dir := range []string{"objects", "refs", "info"} {
		if err := os.Mkdir(filepath.Join(view, dir), 0700); err != nil {
			return "", errors.Join(err, os.RemoveAll(view))
		}
	}
	return view, nil
}

// The detached HEAD needs no copy of mutable refs, remote configuration or
// reflogs. An unborn HEAD still permits an index/worktree comparison.
func (q repositoryQuery) installReadHead(view string) error {
	r, err := q.run("rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return err
	}
	var head []byte
	switch r.ExitCode {
	case 0:
		head = r.Stdout
	case 1:
		head = []byte("ref: refs/heads/unborn\n")
	default:
		return fmt.Errorf("git: resolve comparison HEAD: %s", r.Stderr)
	}
	return os.WriteFile(filepath.Join(view, "HEAD"), head, 0600)
}

// Only display and index interpretation settings cross into the comparison
// repository. A config rewrite between discovery and execution cannot add a
// command, hook, remote or include to this closed list.
func (q repositoryQuery) installReadConfig(view string, env []string) ([]string, error) {
	r, err := q.run("config", "--null", "--get-regexp", `^(core\.(filemode|ignorecase|symlinks|autocrlf|eol|safecrlf|trustctime|checkstat|precomposeunicode|excludesfile|attributesfile)|extensions\.objectformat)$`)
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
	if err := os.WriteFile(filepath.Join(view, "config"), []byte(config), 0600); err != nil {
		return nil, err
	}
	return append(env, "GIT_CONFIG_COUNT="+strconv.Itoa(count)), nil
}

func copyReadMaterial(source, destination string) (err error) {
	// Git replaces an index through a lock and rename. A link therefore keeps
	// split-index identity and avoids copying large indexes on the same volume.
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
	if err := os.Link(source, destination); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
