package runtimebinding

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

type Protocol struct {
	Version string `json:"version"`
}

type Server struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	DefaultWorkspace string `json:"defaultWorkspace"`
	Home             string `json:"home"`
}

type Feature struct {
	Enabled               bool `json:"enabled"`
	ClientOptIn           bool `json:"clientOptIn"`
	ClientRequested       bool `json:"clientRequested"`
	RequiredByRunProtocol bool `json:"requiredByRunProtocol"`
}

// Available reports whether the server and this client agreed to use the
// feature. Server support alone is insufficient for opt-in capabilities.
func (f Feature) Available() bool {
	return f.Enabled && (!f.ClientOptIn || f.ClientRequested)
}

type ReplayLimits struct {
	Scope     string `json:"scope"`
	MaxEvents int    `json:"maxEvents"`
	MaxBytes  int    `json:"maxBytes"`
}

type Limits struct {
	RunConcurrency                   RunConcurrencyLimit         `json:"runConcurrency"`
	CommandReplay                    commandreplay.Capability    `json:"commandReplay"`
	RunReplay                        ReplayLimits                `json:"runReplay"`
	MCPAuthorizationRetentionSeconds int                         `json:"mcpAuthorizationRetentionSeconds"`
	RuntimeSubscription              protocol.SubscriptionLimits `json:"runtimeSubscription"`
}

// Profile is the complete, CLI-owned projection of one successful runtime
// discovery. It is immutable by convention; Clone crosses ownership boundaries.
type Profile struct {
	Protocol         Protocol           `json:"protocol"`
	Server           Server             `json:"server"`
	RunEvents        []string           `json:"runEvents"`
	RuntimeTopics    []string           `json:"runtimeTopics"`
	StreamingMethods []string           `json:"streamingMethods"`
	Features         map[string]Feature `json:"features"`
	Limits           Limits             `json:"limits"`
}

func (p Profile) Clone() Profile {
	p.RunEvents = slices.Clone(p.RunEvents)
	p.RuntimeTopics = slices.Clone(p.RuntimeTopics)
	p.StreamingMethods = slices.Clone(p.StreamingMethods)
	p.Features = maps.Clone(p.Features)
	return p
}

func (p Profile) Validate() error {
	var problems []error
	if strings.TrimSpace(p.Protocol.Version) == "" {
		problems = append(problems, errors.New("protocol version is empty"))
	}
	if strings.TrimSpace(p.Server.Name) == "" || strings.TrimSpace(p.Server.Version) == "" {
		problems = append(problems, errors.New("server identity is incomplete"))
	}
	if strings.TrimSpace(p.Server.DefaultWorkspace) == "" || strings.TrimSpace(p.Server.Home) == "" {
		problems = append(problems, errors.New("server filesystem context is incomplete"))
	}
	for name, values := range map[string][]string{
		"run events": p.RunEvents, "runtime topics": p.RuntimeTopics,
		"streaming methods": p.StreamingMethods,
	} {
		if err := validateUniqueStrings(name, values); err != nil {
			problems = append(problems, err)
		}
	}
	for name := range p.Features {
		if strings.TrimSpace(string(name)) == "" {
			problems = append(problems, errors.New("feature name is empty"))
		}
	}
	if err := p.Limits.validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("runtime profile: %w", err)
	}
	return nil
}

func (l Limits) validate() error {
	if err := l.RunConcurrency.Validate(); err != nil {
		return fmt.Errorf("runtime limits: %w", err)
	}
	if err := l.CommandReplay.Validate(); err != nil {
		return fmt.Errorf("runtime limits: %w", err)
	}
	if l.MCPAuthorizationRetentionSeconds <= 0 {
		return errors.New("runtime limits require positive MCP authorization retention")
	}
	if strings.TrimSpace(l.RunReplay.Scope) == "" || l.RunReplay.MaxEvents <= 0 || l.RunReplay.MaxBytes <= 0 {
		return errors.New("runtime replay limits are incomplete")
	}
	if err := l.RuntimeSubscription.ValidateWire(); err != nil {
		return fmt.Errorf("runtime limits: %w", err)
	}
	return nil
}

func validateUniqueStrings(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s item %d is empty", name, index+1)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeat %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (p Profile) Supports(feature string) bool {
	return p.Features[feature].Available()
}

func (p Profile) SupportsRuntimeTopic(topic string) bool {
	return slices.Contains(p.RuntimeTopics, topic)
}

func (p Profile) AvailableFeatureNames() []string {
	names := make([]string, 0, len(p.Features))
	for name, feature := range p.Features {
		if feature.Available() {
			names = append(names, string(name))
		}
	}
	slices.Sort(names)
	return names
}
