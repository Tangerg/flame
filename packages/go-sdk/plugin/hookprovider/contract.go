// Package hookprovider defines installed lifecycle rule sources and command
// execution. Runtime retains trust, event ordering and final authorization.
package hookprovider

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

const (
	MaxCommandInputBytes  = 512 << 10
	MaxCommandOutputBytes = 64 << 10
)

type Discovery struct {
	CWD            string `json:"cwd"`
	ProjectRoot    string `json:"projectRoot"`
	Home           string `json:"home,omitempty"`
	IncludeProject bool   `json:"includeProject"`
}

func (d Discovery) Validate() error {
	if !filepath.IsAbs(d.CWD) || !filepath.IsAbs(d.ProjectRoot) || (d.Home != "" && !filepath.IsAbs(d.Home)) {
		return errors.New("hooks: absolute discovery directories are required")
	}
	relative, err := filepath.Rel(d.ProjectRoot, d.CWD)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("hooks: directory is outside the captured project root")
	}
	return nil
}

type Command struct {
	Command       string     `json:"command"`
	CWD           string     `json:"cwd"`
	Input         hook.Input `json:"input"`
	TimeoutMillis int64      `json:"timeoutMillis" jsonschema:"minimum=1"`
}

func (c Command) Validate() error {
	if c.Command == "" || !utf8.ValidString(c.Command) || !filepath.IsAbs(c.CWD) || c.Input.CWD != c.CWD || c.TimeoutMillis <= 0 || c.TimeoutMillis > hook.MaxTimeoutMillis {
		return errors.New("hooks: invalid command envelope")
	}
	return errors.Join(c.Input.ValidateCommandMaterial(), (hook.Hook{Event: c.Input.Event, Command: c.Command, TimeoutMillis: int(c.TimeoutMillis)}).Validate())
}

type FailureKind string

const (
	Failed   FailureKind = "failed"
	Canceled FailureKind = "canceled"
	TimedOut FailureKind = "timedOut"
)

func (FailureKind) JSONSchemaEnum() []string {
	return []string{string(Failed), string(Canceled), string(TimedOut)}
}

type Failure struct {
	Kind    FailureKind `json:"kind"`
	Message string      `json:"message"`
}

type Result struct {
	Decision *hook.CommandDecision `json:"decision,omitzero"`
	Stderr   string                `json:"stderr,omitempty"`
	ExitCode int                   `json:"exitCode"`
	Failure  *Failure              `json:"failure,omitzero"`
}

func (r Result) Validate() error {
	if r.ExitCode == 0 && r.Failure == nil && (r.Decision == nil || !r.Decision.Verdict.Valid()) {
		return errors.New("hooks: successful command requires a valid decision")
	}
	if r.Decision != nil && !r.Decision.Verdict.Valid() {
		return errors.New("hooks: invalid command decision")
	}
	if r.Decision != nil && len(r.Decision.Reason)+len(r.Decision.InjectContext)+len(r.Decision.RewriteArguments) > MaxCommandOutputBytes {
		return errors.New("hooks: command decision exceeds its output envelope")
	}
	if r.Failure != nil && ((r.Failure.Kind != Failed && r.Failure.Kind != Canceled && r.Failure.Kind != TimedOut) || r.Failure.Message == "") {
		return errors.New("hooks: invalid command failure")
	}
	if len(r.Stderr) > MaxCommandOutputBytes {
		return errors.New("hooks: command diagnostics exceed their output envelope")
	}
	return nil
}

func DiscoverMethod() (*qimumcp.Method[Discovery, []hook.Hook], error) {
	return qimumcp.NewMethod[Discovery, []hook.Hook](qimumcp.MethodConfig{Name: "flame.hooks/discover.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read ordered lifecycle rules. Exclude all project configuration when the Runtime has not admitted its trust."})
}

func ExecuteMethod() (*qimumcp.Method[Command, Result], error) {
	return qimumcp.NewMethod[Command, Result](qimumcp.MethodConfig{Name: "flame.hooks/execute.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Execute one trusted lifecycle command under its original receipt, bounded input, output and process lifetime. Return advice; never grant Runtime authorization."})
}

func Contract() (*portable.Capability, error) {
	discover, err := DiscoverMethod()
	if err != nil {
		return nil, err
	}
	execute, err := ExecuteMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.hooks.v1", Composition: portable.Selection, Scope: extension.ScopeInstallation}, discover.Descriptor(), execute.Descriptor())
}
