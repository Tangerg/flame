package plugin

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/resourceid"
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

// Record is the persisted projection of an installation. It names releases
// by digest only: the admitted release catalog alone owns their content.
type Record struct {
	ID              resourceid.InstallationID
	Source          string
	Selected        fingerprint.Digest
	Staged          *fingerprint.Digest
	Enabled         bool
	Values          map[string]string
	DisabledServers []mcpserver.ServerName
	DisabledSkills  []string
}

// ValueChange is one closed change to a configured input: set it to a value,
// or clear it. The zero value is neither and is refused.
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

// ComponentChange enables or disables one declared component.
type ComponentChange string

const (
	EnableComponent  ComponentChange = "enable"
	DisableComponent ComponentChange = "disable"
)

// Configuration is a delta: members it does not name keep their current
// value or enablement, so a client never has to echo state it cannot read,
// such as a configured secret.
type Configuration struct {
	Values  map[string]ValueChange
	Servers map[mcpserver.ServerName]ComponentChange
	Skills  map[string]ComponentChange
}

// Snapshot binds installation state to the immutable releases it names. It is
// a detached read projection; only Installation transitions advance admission.
// Store readers must hydrate all three values from the same consistent read.
type Snapshot struct {
	Installation *Installation
	Selected     Release
	Staged       *Release
}

// Installation alone advances selection, desired enablement, and configuration. A
// transition that depends on release content is handed the release read from
// the admitted catalog and refuses any release other than the one it names.
// Construction and restoration enforce the same release-bound invariants;
// storage records cannot originate an installation that transitions would reject.
type Installation struct{ record Record }

func New(id resourceid.InstallationID, source string, release Release) (*Installation, error) {
	return Restore(Record{ID: id, Source: source, Selected: release.Digest(), Values: map[string]string{}}, release, nil)
}

// Restore validates persisted state against its admitted release definitions.
// A record alone cannot prove configuration, enablement or staged package identity.
func Restore(r Record, selected Release, staged *Release) (*Installation, error) {
	if err := r.ID.Validate(); err != nil {
		return nil, fmt.Errorf("%w: installation identity: %w", ErrInvalid, err)
	}
	if r.Source == "" || len(r.Source) > 4096 || !utf8.ValidString(r.Source) {
		return nil, fmt.Errorf("%w: installation source", ErrInvalid)
	}
	if err := r.Selected.Validate(); err != nil {
		return nil, fmt.Errorf("%w: selected release: %w", ErrInvalid, err)
	}
	if selected.Digest() != r.Selected {
		return nil, fmt.Errorf("%w: selected release binding", ErrStale)
	}
	if r.Staged != nil {
		if err := r.Staged.Validate(); err != nil {
			return nil, fmt.Errorf("%w: staged release: %w", ErrInvalid, err)
		}
		if staged == nil || staged.Digest() != *r.Staged {
			return nil, fmt.Errorf("%w: staged release binding", ErrStale)
		}
		if err := validateStaged(selected, *staged); err != nil {
			return nil, err
		}
	} else if staged != nil {
		return nil, fmt.Errorf("%w: unexpected staged release", ErrStale)
	}
	if err := validateValues(selected, r.Values); err != nil {
		return nil, err
	}
	for _, name := range r.DisabledServers {
		if err := name.Validate(); err != nil {
			return nil, fmt.Errorf("%w: disabled server: %w", ErrInvalid, err)
		}
		if _, declared := selected.server(name); !declared {
			return nil, fmt.Errorf("%w: unknown disabled server %q", ErrInvalid, name)
		}
	}
	for _, name := range r.DisabledSkills {
		if !selected.declaresSkill(name) {
			return nil, fmt.Errorf("%w: unknown disabled skill %q", ErrInvalid, name)
		}
	}
	if duplicateNames(r.DisabledServers) || duplicateNames(r.DisabledSkills) {
		return nil, fmt.Errorf("%w: duplicate disabled component", ErrInvalid)
	}
	if r.Enabled {
		if err := requiredValues(selected, r.Values, r.DisabledServers); err != nil {
			return nil, err
		}
	}
	i := &Installation{record: clone(r)}
	if i.record.Values == nil {
		i.record.Values = map[string]string{}
	}
	sortServers(i.record.DisabledServers)
	slices.Sort(i.record.DisabledSkills)
	return i, nil
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

func (i *Installation) Snapshot() Record              { return clone(i.record) }
func (i *Installation) ID() resourceid.InstallationID { return i.record.ID }
func (i *Installation) Selected() fingerprint.Digest  { return i.record.Selected }
func (i *Installation) Enabled() bool                 { return i.record.Enabled }
func (i *Installation) Staged() (fingerprint.Digest, bool) {
	if i.record.Staged == nil {
		return fingerprint.Digest{}, false
	}
	return *i.record.Staged, true
}

func (i *Installation) bound(release Release) error {
	if release.Digest() != i.record.Selected {
		return fmt.Errorf("%w: release %s is not selected %s", ErrStale, release.Digest(), i.record.Selected)
	}
	return nil
}

// Stage records a candidate of the same package without touching the
// selected release, its configuration, or its desired enablement.
func (i *Installation) Stage(selected, candidate Release) error {
	if err := i.bound(selected); err != nil {
		return err
	}
	if err := validateStaged(selected, candidate); err != nil {
		return err
	}
	digest := candidate.Digest()
	i.record.Staged = &digest
	return nil
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

// Select is the explicit trust decision for the staged release.
// Configuration survives only where its meaning is unchanged: a value keeps
// its input declaration, a secret additionally keeps its recipient, and a
// disabled component is still declared.
func (i *Installation) Select(selected, staged Release) error {
	if err := i.bound(selected); err != nil {
		return err
	}
	if i.record.Staged == nil || *i.record.Staged != staged.Digest() {
		return fmt.Errorf("%w: release %s is not staged", ErrStale, staged.Digest())
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
			return err
		}
	}
	i.record = next
	return nil
}

// retainsInput keeps a value only for an input the candidate binds to the
// same slot with the same secrecy; whether it is required may change. A secret
// additionally requires the same recipient, so a credential never follows a
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

func (i *Installation) Enable(release Release) error {
	if err := i.bound(release); err != nil {
		return err
	}
	if err := requiredValues(release, i.record.Values, i.record.DisabledServers); err != nil {
		return err
	}
	i.record.Enabled = true
	return nil
}

func (i *Installation) Disable() { i.record.Enabled = false }

func (i *Installation) Configure(release Release, configuration Configuration) error {
	if err := i.bound(release); err != nil {
		return err
	}
	values := maps.Clone(i.record.Values)
	for key, change := range configuration.Values {
		if _, declared := release.input(key); !declared {
			return fmt.Errorf("%w: unknown input %q", ErrInvalid, key)
		}
		switch change.kind {
		case setValue:
			values[key] = change.value
		case clearValue:
			delete(values, key)
		default:
			return fmt.Errorf("%w: input %q change", ErrInvalid, key)
		}
	}
	if err := validateValues(release, values); err != nil {
		return err
	}
	servers, err := applyComponentChanges(i.record.DisabledServers, configuration.Servers, func(name mcpserver.ServerName) bool {
		_, declared := release.server(name)
		return declared
	}, mcpserver.ServerName.String)
	if err != nil {
		return fmt.Errorf("server enablement: %w", err)
	}
	skills, err := applyComponentChanges(i.record.DisabledSkills, configuration.Skills, release.declaresSkill, func(name string) string { return name })
	if err != nil {
		return fmt.Errorf("skill enablement: %w", err)
	}
	if i.record.Enabled {
		if err := requiredValues(release, values, servers); err != nil {
			return err
		}
	}
	i.record.Values = values
	i.record.DisabledServers = servers
	i.record.DisabledSkills = skills
	return nil
}

// applyComponentChanges returns the sorted disabled set after changes, each
// naming a component the release declares.
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
	if len(value) > 8192 {
		return fmt.Errorf("%w: input %q length", ErrInvalid, key)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("%w: input %q encoding", ErrInvalid, key)
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

// ServerAuthority binds tool policy to the admitted code of one server: the
// selected release digest and the server's name in it. Any release change
// makes standing approvals for its tools stale; credentials and availability
// affect connection realization, not permission.
func (i *Installation) ServerAuthority(server mcpserver.ServerName) fingerprint.Digest {
	return fingerprint.Strings(i.record.Selected.String(), server.String())
}

// ServerID names one declared server of this installation in the MCP registry.
func (i *Installation) ServerID(server mcpserver.ServerName) (mcpserver.ID, error) {
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

// ServerIDs names every server the selected release declares.
func (i *Installation) ServerIDs(release Release) ([]mcpserver.ID, error) {
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

// ServerSource binds a declared server's registry record to the selected
// release, its tool authority, and the recipient its credentials follow.
func (i *Installation) ServerSource(release Release, server mcpserver.ServerName) (mcpserver.Source, error) {
	if err := i.bound(release); err != nil {
		return mcpserver.Source{}, err
	}
	recipient, found := release.recipient(server)
	if !found {
		return mcpserver.Source{}, fmt.Errorf("%w: release %s does not declare server %q", ErrInvalid, release.Digest(), server)
	}
	return mcpserver.InstallationSource(i.record.ID, i.record.Selected, i.ServerAuthority(server), recipient)
}

func (i *Installation) authorizePresentation(release Release, digest fingerprint.Digest) error {
	if i.Selected() != digest || release.Digest() != digest {
		return fmt.Errorf("%w: presentation release", ErrStale)
	}
	if !i.Enabled() {
		return fmt.Errorf("%w: presentation requires an enabled installation", ErrDisabled)
	}
	return nil
}

func (i *Installation) ServerEnabled(name mcpserver.ServerName) bool {
	return i.Enabled() && !slices.Contains(i.record.DisabledServers, name)
}

// InputPresence is what a reader may learn about one declared input.
type InputPresence string

const (
	InputUnset      InputPresence = "unset"
	InputConfigured InputPresence = "configured"
	InputValue      InputPresence = "value"
)

// InputState is one input as a view discloses it. Only the installation
// constructs it, so a state that carries a secret's text cannot exist.
type InputState struct {
	presence InputPresence
	value    string
}

func (s InputState) Presence() InputPresence { return s.presence }

// Value is the configured text of an input whose presence is [InputValue].
func (s InputState) Value() (string, bool) { return s.value, s.presence == InputValue }

// View is what any reader outside persistence may see of an installation. A
// configured secret discloses only that it is configured: its text is never
// copied into a view, so no encoder, log or client cache can carry it.
type View struct {
	ID              resourceid.InstallationID
	Source          string
	Enabled         bool
	Inputs          map[string]InputState
	DisabledServers []mcpserver.ServerName
	DisabledSkills  []string
}

func (i *Installation) View(release Release) (View, error) {
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

// Dependency names the release an execution admits from this installation now.
func (i *Installation) Dependency() Dependency {
	return Dependency{InstallationID: i.record.ID, Digest: i.record.Selected}
}

// EnabledSkills projects admitted metadata from the selected release: none
// unless the installation is enabled, and never a Skill the user disabled.
func (i *Installation) EnabledSkills(release Release) ([]Skill, error) {
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

// ServerInputs is where the configured values of one server's inputs go when
// its descriptor is realized.
type ServerInputs struct {
	Env           map[string]string
	Headers       map[string]string
	Authorization string
}

func (i *Installation) ServerInputs(release Release, server mcpserver.ServerName) (ServerInputs, error) {
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

// Staging does not change the selected capabilities execution may admit.
func (i *Installation) ExecutionChanged(previous *Installation) bool {
	return i.record.Selected != previous.record.Selected ||
		i.Enabled() != previous.Enabled() ||
		!maps.Equal(i.record.Values, previous.record.Values) ||
		!slices.Equal(i.record.DisabledServers, previous.record.DisabledServers) ||
		!slices.Equal(i.record.DisabledSkills, previous.record.DisabledSkills)
}

// ChangedServers identifies connection facts advanced by the transition from
// previous, whose selected release is before, to this installation, whose
// selected release is after. Staging does not invalidate a live connection.
func (i *Installation) ChangedServers(previous *Installation, before, after Release) ([]mcpserver.ServerName, error) {
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

func (i *Installation) sameInputValues(previous *Installation, release Release, name mcpserver.ServerName) bool {
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
