package plugin

import (
	"errors"
	"fmt"
	"maps"
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

// State is the closed trust state of an installation. Approval always names
// the selected release: selecting other bytes returns to [Unapproved], so
// changed code is never run without a renewed review.
type State string

const (
	Unapproved State = "unapproved"
	Approved   State = "approved"
	Enabled    State = "enabled"
)

func (s State) validate() error {
	switch s {
	case Unapproved, Approved, Enabled:
		return nil
	default:
		return fmt.Errorf("%w: installation state %q", ErrInvalid, s)
	}
}

// Record is the persisted projection of an installation. It names releases
// by digest only: the admitted release catalog alone owns their content.
type Record struct {
	ID              resourceid.InstallationID
	Source          string
	Selected        fingerprint.Digest
	Staged          *fingerprint.Digest
	State           State
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

// Installation alone advances selection, trust, and configuration. A
// transition that depends on release content is handed the release read from
// the admitted catalog and refuses any release other than the one it names.
// Facts checked against a release when written stay valid, because a release
// never changes under its digest; Restore therefore checks only the facts
// that do not depend on release content.
type Installation struct{ record Record }

func New(id resourceid.InstallationID, source string, release Release) (*Installation, error) {
	return Restore(Record{ID: id, Source: source, Selected: release.Digest(), State: Unapproved, Values: map[string]string{}})
}

func Restore(r Record) (*Installation, error) {
	if err := r.ID.Validate(); err != nil {
		return nil, fmt.Errorf("%w: installation identity: %w", ErrInvalid, err)
	}
	if r.Source == "" || len(r.Source) > 4096 || !utf8.ValidString(r.Source) {
		return nil, fmt.Errorf("%w: installation source", ErrInvalid)
	}
	if err := r.Selected.Validate(); err != nil {
		return nil, fmt.Errorf("%w: selected release: %w", ErrInvalid, err)
	}
	if r.Staged != nil {
		if err := r.Staged.Validate(); err != nil {
			return nil, fmt.Errorf("%w: staged release: %w", ErrInvalid, err)
		}
		if *r.Staged == r.Selected {
			return nil, fmt.Errorf("%w: staged release is already selected", ErrStale)
		}
	}
	if err := r.State.validate(); err != nil {
		return nil, err
	}
	for key, value := range r.Values {
		if err := validateValueText(key, value); err != nil {
			return nil, err
		}
	}
	for _, name := range r.DisabledServers {
		if err := name.Validate(); err != nil {
			return nil, fmt.Errorf("%w: disabled server: %w", ErrInvalid, err)
		}
	}
	if duplicateNames(r.DisabledServers) || duplicateNames(r.DisabledSkills) {
		return nil, fmt.Errorf("%w: duplicate disabled component", ErrInvalid)
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
func (i *Installation) State() State                  { return i.record.State }
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
// selected release, its configuration, or its trust.
func (i *Installation) Stage(selected, candidate Release) error {
	if err := i.bound(selected); err != nil {
		return err
	}
	if candidate.Name() != selected.Name() {
		return fmt.Errorf("%w: staged package identity", ErrInvalid)
	}
	if candidate.Digest() == i.record.Selected {
		return fmt.Errorf("%w: release is already selected", ErrStale)
	}
	digest := candidate.Digest()
	i.record.Staged = &digest
	return nil
}

// Select replaces the selected release with the staged one. The approval
// named the previous bytes, so the installation returns to [Unapproved].
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
	next := Record{ID: i.record.ID, Source: i.record.Source, Selected: staged.Digest(), State: Unapproved, Values: map[string]string{}}
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

// Approve admits the exact selected release. Approving an approved or enabled
// installation changes nothing.
func (i *Installation) Approve(release Release) error {
	if err := i.bound(release); err != nil {
		return err
	}
	if i.record.State == Unapproved {
		i.record.State = Approved
	}
	return nil
}

func (i *Installation) Enable(release Release) error {
	if err := i.bound(release); err != nil {
		return err
	}
	if i.record.State == Unapproved {
		return fmt.Errorf("%w: enable release %s", ErrUnapproved, i.record.Selected)
	}
	if err := requiredValues(release, i.record.Values, i.record.DisabledServers); err != nil {
		return err
	}
	i.record.State = Enabled
	return nil
}

// Disable withdraws enablement and keeps the approval of the selected release.
func (i *Installation) Disable() {
	if i.record.State == Enabled {
		i.record.State = Approved
	}
}

func (i *Installation) Revoke() { i.record.State = Unapproved }

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
	if i.record.State == Enabled {
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

func (i *Installation) Active() bool { return i.record.State == Enabled }

func (i *Installation) ServerEnabled(name mcpserver.ServerName) bool {
	return i.Active() && !slices.Contains(i.record.DisabledServers, name)
}

func (i *Installation) SkillEnabled(name string) bool {
	return i.Active() && !slices.Contains(i.record.DisabledSkills, name)
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
