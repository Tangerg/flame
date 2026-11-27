package main

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	toolshell "github.com/Tangerg/scope/tools/shell"
)

const (
	projectChecksVariable = "GITHUB_REVIEW_CHECKS"
	projectCheckLogBytes  = 2048
)

type projectCheckCommand struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type projectCheckPolicy struct {
	Environment   string                `json:"environment"`
	ReadOnlyPaths []string              `json:"readOnlyPaths,omitzero"`
	Checks        []projectCheckCommand `json:"checks"`
	digest        string
}

func readCheckPolicies(value string) (map[string]projectCheckPolicy, error) {
	if value == "" {
		return nil, nil
	}
	var declaration map[string]projectCheckPolicy
	if err := json.Unmarshal([]byte(value), &declaration, json.RejectUnknownMembers(true)); err != nil || len(declaration) == 0 {
		return nil, errors.New("github: review checks must declare policies keyed by repository")
	}
	policies := make(map[string]projectCheckPolicy, len(declaration))
	for name, policy := range declaration {
		repo, err := newRepository(name)
		if _, duplicate := policies[repo.String()]; err != nil || duplicate {
			return nil, errors.New("github: review check policies require distinct repository names")
		}
		if err := policy.seal(); err != nil {
			return nil, fmt.Errorf("github: review check policy for %s: %w", repo.String(), err)
		}
		policies[repo.String()] = policy
	}
	return policies, nil
}

func (p *projectCheckPolicy) seal() error {
	if strings.TrimSpace(p.Environment) == "" || len(p.Environment) > 128 || strings.ContainsFunc(p.Environment, unicode.IsControl) {
		return errors.New("a versioned environment name is required")
	}
	if len(p.Checks) == 0 || len(p.Checks) > 4 || len(p.ReadOnlyPaths) > 8 {
		return errors.New("declare one to four checks and up to eight read-only toolchain paths")
	}
	for i, path := range p.ReadOnlyPaths {
		if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) {
			return errors.New("toolchain paths must be absolute paths below the filesystem root")
		}
		physical, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Dir(physical) == physical {
			return errors.New("toolchain paths must resolve below the filesystem root")
		}
		p.ReadOnlyPaths[i] = physical
	}
	names := make(map[string]bool, len(p.Checks))
	for _, check := range p.Checks {
		if strings.TrimSpace(check.Name) == "" || len(check.Name) > 128 || strings.ContainsFunc(check.Name, unicode.IsControl) || names[check.Name] ||
			strings.TrimSpace(check.Command) == "" || len(check.Command) > 1024 || strings.ContainsRune(check.Command, 0) ||
			check.TimeoutSeconds < 1 || check.TimeoutSeconds > 300 {
			return errors.New("checks need distinct names, commands and timeouts between 1 and 300 seconds")
		}
		names[check.Name] = true
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	p.digest = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return nil
}

func (p projectCheckPolicy) capture() *wire.ProjectVerification {
	if p.digest == "" {
		return nil
	}
	result := &wire.ProjectVerification{
		Environment: p.Environment, PolicyDigest: p.digest,
		Runner: "seatbelt-project-v1 (" + runtime.GOOS + "/" + runtime.GOARCH + ")",
		Checks: make([]wire.ProjectCheck, 0, len(p.Checks)),
	}
	for _, check := range p.Checks {
		result.Checks = append(result.Checks, wire.ProjectCheck{
			Name: check.Name, Command: check.Command, TimeoutSeconds: check.TimeoutSeconds, Status: wire.ProjectCheckPending,
		})
	}
	return result
}

func (p projectCheckPolicy) run(ctx context.Context, scratch, checkout string, evidence *wire.ProjectVerification) {
	if evidence == nil {
		return
	}
	granted := p.capture()
	if granted == nil || evidence.PolicyDigest != granted.PolicyDigest || evidence.Runner != granted.Runner {
		finishPendingProjectChecks(evidence, wire.ProjectCheckUnavailable, "The captured check policy or runner is no longer configured.")
		return
	}
	evidence.Checks = granted.Checks
	for i := range evidence.Checks {
		if ctx.Err() != nil {
			return
		}
		runProjectCheck(ctx, scratch, checkout, p.ReadOnlyPaths, &evidence.Checks[i])
	}
}

func copyProjectChecks(original *wire.ProjectVerification) *wire.ProjectVerification {
	if original == nil {
		return nil
	}
	clone := *original
	clone.Checks = slices.Clone(original.Checks)
	return &clone
}

func finishPendingProjectChecks(evidence *wire.ProjectVerification, status wire.ProjectCheckStatus, reason string) {
	if evidence == nil {
		return
	}
	for i := range evidence.Checks {
		check := &evidence.Checks[i]
		if check.Status == wire.ProjectCheckPending {
			check.Status, check.Log = status, reason
		}
	}
}

func runProjectCheck(ctx context.Context, scratch, checkout string, readOnly []string, check *wire.ProjectCheck) {
	limited, cancel := context.WithTimeout(ctx, time.Duration(check.TimeoutSeconds)*time.Second)
	defer cancel()
	started := time.Now()
	workspace, err := sandbox.New(limited, sandbox.Config{BaseDir: scratch, ReadOnlyPaths: readOnly, RestrictReads: true}, checkout)
	var output toolshell.Output
	if err == nil {
		output, err = workspace.Run(limited, toolshell.Input{Cmd: check.Command})
		if err == nil && !output.CancellationObserved {
			check.ExitCode = new(output.ExitCode)
		}
		err = errors.Join(err, workspace.Shutdown())
	}
	check.DurationMillis = new(time.Since(started).Milliseconds())
	switch {
	case errors.Is(limited.Err(), context.DeadlineExceeded) && (output.CancellationObserved || err != nil):
		check.Status = wire.ProjectCheckTimedOut
	case err != nil:
		check.Status = wire.ProjectCheckUnavailable
	case output.ExitCode != 0:
		check.Status = wire.ProjectCheckFailed
	default:
		check.Status = wire.ProjectCheckPassed
	}
	log := string(output.Stdout)
	if len(output.Stderr) != 0 {
		log += "\n" + string(output.Stderr)
	}
	if err != nil {
		log += "\n" + err.Error()
	}
	check.Log, check.LogTruncated = limitReviewEvidence(strings.TrimSpace(log), projectCheckLogBytes)
}

func limitReviewEvidence(value string, limit int) (string, bool) {
	text := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return '�'
		}
		return r
	}, value)
	output := capture.NewWriter(limit)
	_, _ = output.Write([]byte(text))
	return output.Diagnostic(), output.Truncated()
}
