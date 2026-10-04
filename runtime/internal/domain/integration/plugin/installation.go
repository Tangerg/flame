package plugin

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const Namespace = "io.github.tangerg.flame"
const APIVersion = 1
const MaxInstallations = 128

var (
	ErrInvalid     = errors.New("plugin: invalid installation")
	ErrUnapproved  = errors.New("plugin: release is not approved")
	ErrStale       = errors.New("plugin: stale release")
	ErrInUse       = errors.New("plugin: release is in use")
	ErrNotFound    = errors.New("plugin: installation not found")
	ErrUnavailable = errors.New("plugin: release is unavailable")
)

type Diagnostic struct {
	Component string
	Code      string
}
type Server struct {
	Name    string
	Type    Transport
	Command string
	Args    []string
	Env     map[string]string
	CWD     string
	URL     string
	Headers map[string]string
}
type Input struct {
	ID       string
	Secret   bool
	Required bool
	Server   string
	Target   InputTarget
	Key      string
}
type RequestGrant struct {
	Capability Capability
	Targets    []string
}

type Theme struct {
	ID     string
	Title  string
	Scheme ThemeScheme
	Colors map[string]string
}
type Skill struct {
	Name        string
	Description string
}
type Release struct {
	Requests    []RequestGrant
	Digest      string
	Name        string
	Version     string
	Description string
	Servers     []Server
	Inputs      []Input
	Themes      []Theme
	Skills      []Skill
	Diagnostics []Diagnostic
}

type Record struct {
	Grants          []RequestGrant
	ID              string
	Source          string
	Selected        Release
	Staged          *Release
	Enabled         bool
	ApprovedDigest  string
	Values          map[string]string
	DisabledServers []string
	DisabledSkills  []string
}

type Configuration struct {
	Digest          string
	Values          map[string]*string
	DisabledServers []string
	DisabledSkills  []string
}

// Installation alone advances selection, trust, and configuration. Persistence
// records and connection descriptors are snapshots, never independent writers.
type Installation struct{ record Record }

func New(id, source string, release Release) (*Installation, error) {
	return Restore(Record{ID: id, Source: source, Selected: release, Values: map[string]string{}})
}
func Restore(r Record) (*Installation, error) {
	_, err := resourceid.ParseInstallation(r.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: installation identity: %w", ErrInvalid, err)
	}
	if r.Source == "" || len(r.Source) > 4096 || !utf8.ValidString(r.Source) {
		return nil, fmt.Errorf("%w: installation source", ErrInvalid)
	}
	if err := r.Selected.Validate(); err != nil {
		return nil, fmt.Errorf("selected release: %w", err)
	}
	if r.Staged != nil {
		if err := r.Staged.Validate(); err != nil {
			return nil, fmt.Errorf("staged release: %w", err)
		}
		if r.Staged.Name != r.Selected.Name {
			return nil, fmt.Errorf("%w: staged package identity", ErrInvalid)
		}
		if r.Staged.Digest == r.Selected.Digest {
			return nil, fmt.Errorf("%w: staged release is already selected", ErrStale)
		}
	}
	if r.ApprovedDigest != "" && r.ApprovedDigest != r.Selected.Digest {
		return nil, ErrStale
	}
	if r.Enabled && r.ApprovedDigest != r.Selected.Digest {
		return nil, ErrUnapproved
	}
	i := &Installation{record: clone(r)}
	if err := validateGrants(r.Selected.Requests, r.Grants); err != nil {
		return nil, fmt.Errorf("installation grants: %w", err)
	}
	if r.ApprovedDigest == "" && len(r.Grants) > 0 {
		return nil, fmt.Errorf("%w: unapproved installation has grants", ErrInvalid)
	}
	i.record.Grants = canonicalGrants(r.Grants)
	if err := i.Configure(Configuration{Digest: r.Selected.Digest, DisabledServers: r.DisabledServers, DisabledSkills: r.DisabledSkills}); err != nil {
		return nil, err
	}
	return i, nil
}

var digestExpression = regexp.MustCompile(DigestPattern)

func ValidDigest(s string) bool { return digestExpression.MatchString(s) }
func clone(r Record) Record {
	r.Selected = r.Selected.Clone()
	if r.Staged != nil {
		v := r.Staged.Clone()
		r.Staged = &v
	}
	r.Values = maps.Clone(r.Values)
	r.DisabledServers = slices.Clone(r.DisabledServers)
	r.DisabledSkills = slices.Clone(r.DisabledSkills)
	r.Grants = slices.Clone(r.Grants)
	for index := range r.Grants {
		r.Grants[index].Targets = slices.Clone(r.Grants[index].Targets)
	}
	return r
}

func (i *Installation) Snapshot() Record { return clone(i.record) }
func (i *Installation) Stage(r Release) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("staged release: %w", err)
	}
	if r.Name != i.record.Selected.Name {
		return fmt.Errorf("%w: staged package identity", ErrInvalid)
	}
	if r.Digest == i.record.Selected.Digest {
		return fmt.Errorf("%w: release is already selected", ErrStale)
	}
	rCopy := clone(Record{Selected: r}).Selected
	i.record.Staged = &rCopy
	return nil
}
func (i *Installation) Select(digest string) error {
	if i.record.Staged == nil || i.record.Staged.Digest != digest {
		return ErrStale
	}
	i.record.Selected = *i.record.Staged
	i.record.Staged = nil
	i.record.Enabled = false
	i.record.ApprovedDigest = ""
	i.record.Grants = nil
	i.record.Values = map[string]string{}
	i.record.DisabledServers = nil
	i.record.DisabledSkills = nil
	return nil
}
func (i *Installation) Approve(digest string, grants []RequestGrant) error {
	if digest != i.record.Selected.Digest {
		return ErrStale
	}
	if err := validateGrants(i.record.Selected.Requests, grants); err != nil {
		return fmt.Errorf("approval grants: %w", err)
	}
	i.record.Grants = canonicalGrants(grants)
	i.record.ApprovedDigest = digest
	return nil
}
func (i *Installation) Enable(enabled bool) error {
	if enabled {
		if i.record.ApprovedDigest != i.record.Selected.Digest {
			return ErrUnapproved
		}
		if err := i.requiredValues(i.record.Values, i.record.DisabledServers); err != nil {
			return err
		}
	}
	i.record.Enabled = enabled
	return nil
}
func (i *Installation) Revoke() {
	i.record.Enabled = false
	i.record.ApprovedDigest = ""
	i.record.Grants = nil
}
func (i *Installation) Configure(configuration Configuration) error {
	if configuration.Digest != i.record.Selected.Digest {
		return ErrStale
	}
	values := maps.Clone(i.record.Values)
	if values == nil {
		values = map[string]string{}
	}
	for key, value := range configuration.Values {
		if !slices.ContainsFunc(i.record.Selected.Inputs, func(input Input) bool { return input.ID == key }) {
			return fmt.Errorf("%w: unknown input %q", ErrInvalid, key)
		}
		if value == nil {
			delete(values, key)
		} else {
			values[key] = *value
		}
	}
	if err := i.validateValues(values); err != nil {
		return err
	}
	for _, name := range configuration.DisabledServers {
		if !slices.ContainsFunc(i.record.Selected.Servers, func(s Server) bool { return s.Name == name }) {
			return fmt.Errorf("%w: unknown disabled server %q", ErrInvalid, name)
		}
	}
	for _, name := range configuration.DisabledSkills {
		if !slices.ContainsFunc(i.record.Selected.Skills, func(s Skill) bool { return s.Name == name }) {
			return fmt.Errorf("%w: unknown disabled skill %q", ErrInvalid, name)
		}
	}
	if i.record.Enabled {
		if err := i.requiredValues(values, configuration.DisabledServers); err != nil {
			return err
		}
	}
	if duplicateNames(configuration.DisabledServers) || duplicateNames(configuration.DisabledSkills) {
		return fmt.Errorf("%w: duplicate disabled component", ErrInvalid)
	}
	i.record.Values = maps.Clone(values)
	i.record.DisabledServers = slices.Clone(configuration.DisabledServers)
	slices.Sort(i.record.DisabledServers)
	i.record.DisabledSkills = slices.Clone(configuration.DisabledSkills)
	slices.Sort(i.record.DisabledSkills)
	return nil
}
func (i *Installation) validateValues(values map[string]string) error {
	for key, value := range values {
		if len(value) > 8192 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || !slices.ContainsFunc(i.record.Selected.Inputs, func(input Input) bool { return input.ID == key }) {
			return fmt.Errorf("%w: input %q", ErrInvalid, key)
		}
	}
	for _, input := range i.record.Selected.Inputs {
		value := values[input.ID]
		var err error
		switch input.Target {
		case Header:
			err = mcpserver.ValidateHTTPHeaders("", map[string]string{input.Key: value})
		case Authorization:
			err = mcpserver.ValidateHTTPHeaders(value, nil)
		}
		if err != nil {
			return fmt.Errorf("%w: input %q: %w", ErrInvalid, input.ID, err)
		}
	}
	return nil
}

func (i *Installation) requiredValues(values map[string]string, disabled []string) error {
	for _, input := range i.record.Selected.Inputs {
		if input.Required && !slices.Contains(disabled, input.Server) && values[input.ID] == "" {
			return fmt.Errorf("%w: required input %q", ErrInvalid, input.ID)
		}
	}
	return nil
}

// ServerAuthority binds tool policy to admitted code and that source's grants.
// Credentials and availability affect connection realization, not permission.
func (i *Installation) ServerAuthority(server string) string {
	var targets []string
	for _, grant := range i.record.Grants {
		if grant.Capability == InvokeTools {
			for _, target := range grant.Targets {
				if strings.HasPrefix(target, server+"/") {
					targets = append(targets, target)
				}
			}
		}
	}
	return fingerprint.Strings(append([]string{i.record.Selected.Digest, i.record.ApprovedDigest}, targets...)...)
}

func (i *Installation) Active() bool {
	return i.record.Enabled && i.record.ApprovedDigest == i.record.Selected.Digest
}

func (i *Installation) ServerEnabled(name string) bool {
	return i.Active() && !slices.Contains(i.record.DisabledServers, name)
}

func (i *Installation) SkillEnabled(name string) bool {
	return i.Active() && !slices.Contains(i.record.DisabledSkills, name)
}

// RetainsServerCredentials decides whether an existing grant survives this
// transition. Returning to an old configuration must not revive its callbacks.
func (i *Installation) RetainsServerCredentials(previous *Installation, name string) bool {
	if i.record.ID != previous.record.ID || i.ServerAuthority(name) != previous.ServerAuthority(name) || previous.ServerEnabled(name) && !i.ServerEnabled(name) {
		return false
	}
	return i.sameInputValues(previous, name)
}

func (i *Installation) sameInputValues(previous *Installation, name string) bool {
	for _, input := range i.record.Selected.Inputs {
		if input.Server != name {
			continue
		}
		value, configured := i.record.Values[input.ID]
		oldValue, previouslyConfigured := previous.record.Values[input.ID]
		if configured != previouslyConfigured || value != oldValue {
			return false
		}
	}
	return true
}

// ChangedServers identifies connection facts advanced by this transition.
// Staging and unrelated source grants do not invalidate a live connection.
func (i *Installation) ChangedServers(previous *Installation) []string {
	var names, changed []string
	for _, release := range []Release{previous.record.Selected, i.record.Selected} {
		for _, server := range release.Servers {
			if !slices.Contains(names, server.Name) {
				names = append(names, server.Name)
			}
		}
	}
	for _, name := range names {
		if i.record.Selected.Digest != previous.record.Selected.Digest ||
			i.ServerAuthority(name) != previous.ServerAuthority(name) ||
			i.ServerEnabled(name) != previous.ServerEnabled(name) {
			changed = append(changed, name)
			continue
		}
		if !i.sameInputValues(previous, name) {
			changed = append(changed, name)
		}
	}
	return changed
}

func canonicalGrants(grants []RequestGrant) []RequestGrant {
	result := clone(Record{Grants: grants}).Grants
	for index := range result {
		slices.Sort(result[index].Targets)
	}
	slices.SortFunc(result, func(a, b RequestGrant) int { return strings.Compare(string(a.Capability), string(b.Capability)) })
	return result
}

func duplicateNames(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}
