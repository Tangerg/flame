package plugin

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

	framevalue "github.com/Tangerg/frame/value"
)

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

func (r Release) InstallationServerIDs(installation identity.InstallationID) ([]mcpserver.ID, error) {
	origin, err := mcpserver.InstallationOrigin(installation)
	if err != nil {
		return nil, err
	}
	result := make([]mcpserver.ID, 0, len(r.declaration.Servers))
	for _, server := range r.declaration.Servers {
		id, err := mcpserver.NewID(origin, server.Name)
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
