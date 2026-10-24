package localruntime

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

//go:embed node-release.json
var nodeReleaseJSON []byte

// Node is the immutable launch projection of the distribution's selection.
// Package bytes never supply an interpreter path or flags.
type Node struct {
	command      string
	distribution string
	identity     string
	expected     string
	arguments    []string
}

func NodeForExecutable(executable string) (Node, error) {
	if !filepath.IsAbs(executable) {
		return Node{}, errors.New("node companion: host executable must be absolute")
	}
	var release struct {
		Version   string   `json:"version"`
		Arguments []string `json:"arguments"`
		Targets   map[string]struct {
			Archive          string `json:"archive"`
			ArchiveSHA256    string `json:"archiveSHA256"`
			Executable       string `json:"executable"`
			ExecutableSHA256 string `json:"executableSHA256"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(nodeReleaseJSON, &release); err != nil {
		return Node{}, fmt.Errorf("node companion: distribution selection: %w", err)
	}
	target, found := release.Targets[runtime.GOOS+"-"+runtime.GOARCH]
	if !found {
		return Node{}, fmt.Errorf("node companion: unsupported distribution target %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	policy := sha256.Sum256([]byte(target.ArchiveSHA256 + "\x00" + strings.Join(release.Arguments, "\x00")))
	return Node{
		distribution: filepath.Dir(executable),
		command:      filepath.Join(filepath.Dir(executable), "runtimes", "node", runtime.GOOS+"-"+runtime.GOARCH, filepath.FromSlash(target.Executable)),
		identity:     "node-" + release.Version + "-" + runtime.GOOS + "-" + runtime.GOARCH + ":" + hex.EncodeToString(policy[:]),
		expected:     target.ExecutableSHA256,
		arguments:    release.Arguments,
	}, nil
}

func (n Node) Command() string                 { return n.command }
func (n Node) Identity() string                { return n.identity }
func (n Node) Arguments(entry string) []string { return append(slices.Clone(n.arguments), entry) }

// Verify refuses missing or altered companions before any plugin is evaluated.
// A deployment is replaced only while quiescent, never underneath this launch.
func (n Node) Verify(ctx context.Context) (err error) {
	if n.command == "" {
		return errors.New("node companion: distribution selection is required")
	}
	root, err := os.OpenRoot(n.distribution)
	if err != nil {
		return fmt.Errorf("node companion: open distribution: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	relative, err := filepath.Rel(n.distribution, n.command)
	if err != nil || !filepath.IsLocal(relative) {
		return errors.New("node companion: executable is outside distribution")
	}
	var current string
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := root.Lstat(current)
		if err != nil {
			return fmt.Errorf("node companion: inspect executable path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("node companion: executable path is redirected")
		}
	}
	file, err := root.Open(relative)
	if err != nil {
		return fmt.Errorf("node companion: open executable: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return errors.New("node companion: executable must be a regular executable file")
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := file.Read(buffer)
		_, _ = hash.Write(buffer[:count])
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("node companion: read executable: %w", readErr)
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != n.expected {
		return errors.New("node companion: selected executable checksum mismatch")
	}
	return nil
}
