package plugin

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

	framevalue "github.com/Tangerg/frame/value"
)

const Namespace = "io.github.tangerg.flame"
const MaxInstallations = 128

var (
	ErrInvalid     = errors.New("plugin: invalid installation")
	ErrDisabled    = errors.New("plugin: installation is disabled")
	ErrStale       = errors.New("plugin: stale release")
	ErrInUse       = errors.New("plugin: release is in use")
	ErrNotFound    = errors.New("plugin: installation not found")
	ErrUnavailable = errors.New("plugin: release is unavailable")
)

type Record struct {
	ID              identity.InstallationID
	Source          string
	Selected        framevalue.Digest
	Staged          *framevalue.Digest
	Enabled         bool
	Values          map[string]string
	DisabledServers []mcpserver.ServerName
	DisabledSkills  []string
}

type ValueChange struct {
	kind  valueChangeKind
	value string
}

type valueChangeKind uint8

const (
	setValue valueChangeKind = iota + 1
	clearValue
)

func SetValue(value string) ValueChange { return ValueChange{kind: setValue, value: value} }
func ClearValue() ValueChange           { return ValueChange{kind: clearValue} }

type ComponentChange string

const (
	EnableComponent  ComponentChange = "enable"
	DisableComponent ComponentChange = "disable"
)

// A delta: unnamed members keep their value, so a client never echoes state it
// cannot read, such as a configured secret.
type Configuration struct {
	Values  map[string]ValueChange
	Servers map[mcpserver.ServerName]ComponentChange
	Skills  map[string]ComponentChange
}

type Snapshot struct {
	Installation Installation
	Selected     Release
	Staged       *Release
}

type Installation struct{ record Record }

func New(id identity.InstallationID, source string, release Release) (Installation, error) {
	return Restore(Record{ID: id, Source: source, Selected: release.Digest(), Values: map[string]string{}}, release, nil)
}

const (
	MaximumSourceCharacters      = 4096
	MaximumValueCharacters       = 8192
	MaximumCommandTextCharacters = 256
	MaximumRoleCharacters        = 256
)

func Restore(r Record, selected Release, staged *Release) (Installation, error) {
	if err := r.ID.Validate(); err != nil {
		return Installation{}, fmt.Errorf("%w: installation identity: %w", ErrInvalid, err)
	}
	if r.Source == "" || !utf8.ValidString(r.Source) || utf8.RuneCountInString(r.Source) > MaximumSourceCharacters {
		return Installation{}, fmt.Errorf("%w: installation source", ErrInvalid)
	}
	if err := r.bindReleases(selected, staged); err != nil {
		return Installation{}, err
	}
	if err := validateValues(selected, r.Values); err != nil {
		return Installation{}, err
	}
	if err := r.validateDisabled(selected); err != nil {
		return Installation{}, err
	}
	if r.Enabled {
		if err := requiredValues(selected, r.Values, r.DisabledServers); err != nil {
			return Installation{}, err
		}
	}
	record := clone(r)
	if record.Values == nil {
		record.Values = map[string]string{}
	}
	sortServers(record.DisabledServers)
	slices.Sort(record.DisabledSkills)
	return Installation{record: record}, nil
}

func (r Record) bindReleases(selected Release, staged *Release) error {
	if err := r.Selected.Validate(); err != nil {
		return fmt.Errorf("%w: selected release: %w", ErrInvalid, err)
	}
	if selected.Digest() != r.Selected {
		return fmt.Errorf("%w: selected release binding", ErrStale)
	}
	if r.Staged == nil {
		if staged != nil {
			return fmt.Errorf("%w: unexpected staged release", ErrStale)
		}
		return nil
	}
	if err := r.Staged.Validate(); err != nil {
		return fmt.Errorf("%w: staged release: %w", ErrInvalid, err)
	}
	if staged == nil || staged.Digest() != *r.Staged {
		return fmt.Errorf("%w: staged release binding", ErrStale)
	}
	return validateStaged(selected, *staged)
}

func (r Record) validateDisabled(selected Release) error {
	for _, name := range r.DisabledServers {
		if err := name.Validate(); err != nil {
			return fmt.Errorf("%w: disabled server: %w", ErrInvalid, err)
		}
		if _, declared := selected.server(name); !declared {
			return fmt.Errorf("%w: unknown disabled server %q", ErrInvalid, name)
		}
	}
	for _, name := range r.DisabledSkills {
		if !selected.declaresSkill(name) {
			return fmt.Errorf("%w: unknown disabled skill %q", ErrInvalid, name)
		}
	}
	if duplicateNames(r.DisabledServers) || duplicateNames(r.DisabledSkills) {
		return fmt.Errorf("%w: duplicate disabled component", ErrInvalid)
	}
	return nil
}

func clone(r Record) Record {
	if r.Staged != nil {
		staged := *r.Staged
		r.Staged = &staged
	}
	r.Values = maps.Clone(r.Values)
	r.DisabledServers = slices.Clone(r.DisabledServers)
	r.DisabledSkills = slices.Clone(r.DisabledSkills)
	return r
}

func (i Installation) Snapshot() Record            { return clone(i.record) }
func (i Installation) ID() identity.InstallationID { return i.record.ID }
func (i Installation) Selected() framevalue.Digest { return i.record.Selected }
func (i Installation) Enabled() bool               { return i.record.Enabled }

func (i Installation) Staged() (framevalue.Digest, bool) {
	if i.record.Staged == nil {
		return framevalue.Digest{}, false
	}
	return *i.record.Staged, true
}

func (i Installation) bound(release Release) error {
	if release.Digest() != i.record.Selected {
		return fmt.Errorf("%w: release %s is not selected %s", ErrStale, release.Digest(), i.record.Selected)
	}
	return nil
}

func (i Installation) Stage(selected, candidate Release) (Installation, error) {
	if err := i.bound(selected); err != nil {
		return Installation{}, err
	}
	if err := validateStaged(selected, candidate); err != nil {
		return Installation{}, err
	}
	next := clone(i.record)
	digest := candidate.Digest()
	next.Staged = &digest
	return Installation{record: next}, nil
}

func validateStaged(selected, candidate Release) error {
	if candidate.Name() != selected.Name() {
		return fmt.Errorf("%w: staged package identity", ErrInvalid)
	}
	if candidate.Digest() == selected.Digest() {
		return fmt.Errorf("%w: release is already selected", ErrStale)
	}
	return nil
}

func (i Installation) Select(selected, staged Release) (Installation, error) {
	if err := i.bound(selected); err != nil {
		return Installation{}, err
	}
	if i.record.Staged == nil || *i.record.Staged != staged.Digest() {
		return Installation{}, fmt.Errorf("%w: release %s is not staged", ErrStale, staged.Digest())
	}
	next := Record{ID: i.record.ID, Source: i.record.Source, Selected: staged.Digest(), Enabled: i.record.Enabled, Values: map[string]string{}}
	for id, value := range i.record.Values {
		if retainsInput(selected, staged, id) {
			next.Values[id] = value
		}
	}
	for _, name := range i.record.DisabledServers {
		if _, found := staged.server(name); found {
			next.DisabledServers = append(next.DisabledServers, name)
		}
	}
	for _, name := range i.record.DisabledSkills {
		if staged.declaresSkill(name) {
			next.DisabledSkills = append(next.DisabledSkills, name)
		}
	}
	if next.Enabled {
		if err := requiredValues(staged, next.Values, next.DisabledServers); err != nil {
			return Installation{}, err
		}
	}
	return Installation{record: next}, nil
}

// A secret also requires the same recipient, so a credential never follows a
// changed endpoint or executable.
func retainsInput(selected, staged Release, id string) bool {
	before, declared := selected.input(id)
	after, redeclared := staged.input(id)
	before.Required, after.Required = false, false
	if !declared || !redeclared || before != after {
		return false
	}
	if !after.Secret {
		return true
	}
	previous, _ := selected.recipient(before.Server)
	current, _ := staged.recipient(after.Server)
	return previous == current
}

func (i Installation) Enable(release Release) (Installation, error) {
	if err := i.bound(release); err != nil {
		return Installation{}, err
	}
	if err := requiredValues(release, i.record.Values, i.record.DisabledServers); err != nil {
		return Installation{}, err
	}
	next := clone(i.record)
	next.Enabled = true
	return Installation{record: next}, nil
}

func (i Installation) Disable() Installation {
	next := clone(i.record)
	next.Enabled = false
	return Installation{record: next}
}

func (i Installation) Configure(release Release, configuration Configuration) (Installation, error) {
	if err := i.bound(release); err != nil {
		return Installation{}, err
	}
	values := maps.Clone(i.record.Values)
	for key, change := range configuration.Values {
		if _, declared := release.input(key); !declared {
			return Installation{}, fmt.Errorf("%w: unknown input %q", ErrInvalid, key)
		}
		switch change.kind {
		case setValue:
			values[key] = change.value
		case clearValue:
			delete(values, key)
		default:
			return Installation{}, fmt.Errorf("%w: input %q change", ErrInvalid, key)
		}
	}
	if err := validateValues(release, values); err != nil {
		return Installation{}, err
	}
	servers, err := applyComponentChanges(i.record.DisabledServers, configuration.Servers, func(name mcpserver.ServerName) bool {
		_, declared := release.server(name)
		return declared
	}, mcpserver.ServerName.String)
	if err != nil {
		return Installation{}, fmt.Errorf("server enablement: %w", err)
	}
	skills, err := applyComponentChanges(i.record.DisabledSkills, configuration.Skills, release.declaresSkill, func(name string) string { return name })
	if err != nil {
		return Installation{}, fmt.Errorf("skill enablement: %w", err)
	}
	if i.record.Enabled {
		if err := requiredValues(release, values, servers); err != nil {
			return Installation{}, err
		}
	}
	next := clone(i.record)
	next.Values = values
	next.DisabledServers = servers
	next.DisabledSkills = skills
	return Installation{record: next}, nil
}

func applyComponentChanges[N comparable](disabled []N, changes map[N]ComponentChange, declared func(N) bool, text func(N) string) ([]N, error) {
	result := slices.Clone(disabled)
	for name, change := range changes {
		if !declared(name) {
			return nil, fmt.Errorf("%w: unknown component %q", ErrInvalid, text(name))
		}
		result = slices.DeleteFunc(result, func(existing N) bool { return existing == name })
		switch change {
		case EnableComponent:
		case DisableComponent:
			result = append(result, name)
		default:
			return nil, fmt.Errorf("%w: component %q change %q", ErrInvalid, text(name), change)
		}
	}
	slices.SortFunc(result, func(a, b N) int { return strings.Compare(text(a), text(b)) })
	return result, nil
}

func validateValueText(key, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("%w: input %q encoding", ErrInvalid, key)
	}
	if utf8.RuneCountInString(value) > MaximumValueCharacters {
		return fmt.Errorf("%w: input %q length", ErrInvalid, key)
	}
	return nil
}

func validateValues(release Release, values map[string]string) error {
	for key, value := range values {
		input, declared := release.input(key)
		if !declared {
			return fmt.Errorf("%w: unknown input %q", ErrInvalid, key)
		}
		if err := validateValueText(key, value); err != nil {
			return err
		}
		var err error
		switch input.Target {
		case Header:
			err = mcpserver.ValidateHTTPHeaders("", map[string]string{input.Key: value})
		case Authorization:
			err = mcpserver.ValidateHTTPHeaders(value, nil)
		case Environment:
		}
		if err != nil {
			return fmt.Errorf("%w: input %q: %w", ErrInvalid, input.ID, err)
		}
	}
	return nil
}

func requiredValues(release Release, values map[string]string, disabled []mcpserver.ServerName) error {
	for _, input := range release.declaration.Inputs {
		if input.Required && !slices.Contains(disabled, input.Server) && values[input.ID] == "" {
			return fmt.Errorf("%w: required input %q", ErrInvalid, input.ID)
		}
	}
	return nil
}

func (i Installation) ServerAuthority(server mcpserver.ServerName) framevalue.Digest {
	return identity.DigestStrings(i.record.Selected.String(), server.String())
}

func (i Installation) ServerID(server mcpserver.ServerName) (mcpserver.ID, error) {
	origin, err := mcpserver.InstallationOrigin(i.record.ID)
	if err != nil {
		return mcpserver.ID{}, fmt.Errorf("plugin: installation %s origin: %w", i.record.ID, err)
	}
	id, err := mcpserver.NewID(origin, server)
	if err != nil {
		return mcpserver.ID{}, fmt.Errorf("plugin: installation %s server %q identity: %w", i.record.ID, server, err)
	}
	return id, nil
}

func (i Installation) ServerIDs(release Release) ([]mcpserver.ID, error) {
	if err := i.bound(release); err != nil {
		return nil, err
	}
	result := make([]mcpserver.ID, 0, len(release.declaration.Servers))
	for _, server := range release.declaration.Servers {
		id, err := i.ServerID(server.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func (i Installation) ServerSource(release Release, server mcpserver.ServerName) (mcpserver.Source, error) {
	if err := i.bound(release); err != nil {
		return mcpserver.Source{}, err
	}
	recipient, found := release.recipient(server)
	if !found {
		return mcpserver.Source{}, fmt.Errorf("%w: release %s does not declare server %q", ErrInvalid, release.Digest(), server)
	}
	return mcpserver.InstallationSource(i.record.ID, i.record.Selected, i.ServerAuthority(server), recipient)
}

func (i Installation) authorizePresentation(release Release, digest framevalue.Digest) error {
	if i.Selected() != digest || release.Digest() != digest {
		return fmt.Errorf("%w: presentation release", ErrStale)
	}
	if !i.Enabled() {
		return fmt.Errorf("%w: presentation requires an enabled installation", ErrDisabled)
	}
	return nil
}

func (i Installation) ServerEnabled(name mcpserver.ServerName) bool {
	return i.Enabled() && !slices.Contains(i.record.DisabledServers, name)
}

type InputPresence string

const (
	InputUnset      InputPresence = "unset"
	InputConfigured InputPresence = "configured"
	InputValue      InputPresence = "value"
)

type InputState struct {
	presence InputPresence
	value    string
}

func (s InputState) Presence() InputPresence { return s.presence }

func (s InputState) Value() (string, bool) { return s.value, s.presence == InputValue }

type View struct {
	ID              identity.InstallationID
	Source          string
	Enabled         bool
	Inputs          map[string]InputState
	DisabledServers []mcpserver.ServerName
	DisabledSkills  []string
}

func (i Installation) View(release Release) (View, error) {
	if err := i.bound(release); err != nil {
		return View{}, err
	}
	inputs := make(map[string]InputState, len(release.declaration.Inputs))
	for _, input := range release.declaration.Inputs {
		value, configured := i.record.Values[input.ID]
		switch {
		case !configured:
			inputs[input.ID] = InputState{presence: InputUnset}
		case input.Secret:
			inputs[input.ID] = InputState{presence: InputConfigured}
		default:
			inputs[input.ID] = InputState{presence: InputValue, value: value}
		}
	}
	return View{
		ID:              i.record.ID,
		Source:          i.record.Source,
		Enabled:         i.record.Enabled,
		Inputs:          inputs,
		DisabledServers: slices.Clone(i.record.DisabledServers),
		DisabledSkills:  slices.Clone(i.record.DisabledSkills),
	}, nil
}

func (i Installation) Dependency() Dependency {
	return Dependency{InstallationID: i.record.ID, Digest: i.record.Selected}
}

func (i Installation) EnabledSkills(release Release) ([]Skill, error) {
	if err := i.bound(release); err != nil {
		return nil, err
	}
	if !i.Enabled() {
		return nil, nil
	}
	var enabled []Skill
	for _, skill := range release.declaration.Skills {
		if !slices.Contains(i.record.DisabledSkills, skill.Name) {
			enabled = append(enabled, skill)
		}
	}
	return enabled, nil
}

type ServerInputs struct {
	Env           map[string]string
	Headers       map[string]string
	Authorization string
}

func (i Installation) ServerInputs(release Release, server mcpserver.ServerName) (ServerInputs, error) {
	if err := i.bound(release); err != nil {
		return ServerInputs{}, err
	}
	result := ServerInputs{Env: map[string]string{}, Headers: map[string]string{}}
	for _, input := range release.declaration.Inputs {
		value, configured := i.record.Values[input.ID]
		if input.Server != server || !configured {
			continue
		}
		switch input.Target {
		case Environment:
			result.Env[input.Key] = value
		case Header:
			result.Headers[input.Key] = value
		case Authorization:
			result.Authorization = value
		}
	}
	return result, nil
}

func (i Installation) ExecutionChanged(previous Installation) bool {
	return i.record.Selected != previous.record.Selected ||
		i.Enabled() != previous.Enabled() ||
		!maps.Equal(i.record.Values, previous.record.Values) ||
		!slices.Equal(i.record.DisabledServers, previous.record.DisabledServers) ||
		!slices.Equal(i.record.DisabledSkills, previous.record.DisabledSkills)
}

func (i Installation) ChangedServers(previous Installation, before, after Release) ([]mcpserver.ServerName, error) {
	if err := previous.bound(before); err != nil {
		return nil, err
	}
	if err := i.bound(after); err != nil {
		return nil, err
	}
	var declared, changed []mcpserver.ServerName
	for _, release := range []Release{before, after} {
		for _, server := range release.declaration.Servers {
			if !slices.Contains(declared, server.Name) {
				declared = append(declared, server.Name)
			}
		}
	}
	for _, name := range declared {
		if before.Digest() != after.Digest() ||
			i.ServerEnabled(name) != previous.ServerEnabled(name) ||
			!i.sameInputValues(previous, after, name) {
			changed = append(changed, name)
		}
	}
	return changed, nil
}

func (i Installation) sameInputValues(previous Installation, release Release, name mcpserver.ServerName) bool {
	for _, input := range release.declaration.Inputs {
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

func sortServers(names []mcpserver.ServerName) {
	slices.SortFunc(names, func(a, b mcpserver.ServerName) int { return strings.Compare(a.String(), b.String()) })
}

func duplicateNames[N comparable](values []N) bool {
	seen := map[N]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}
