package operation

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// Actor identifies the managed code making a nested call. It is captured by
// the connection owner and never decoded from a plugin's business arguments.
type Actor struct {
	installation identity.InstallationID
	release      identity.Digest
	authority    identity.Digest
	server       mcpserver.ServerName
	bindings     *plugin.RequiredBindings
}

func NewActor(installation, release, authority, server string) (Actor, error) {
	id, err := identity.ParseInstallation(installation)
	if err != nil {
		return Actor{}, err
	}
	digest, err := identity.ParseDigest(release)
	if err != nil {
		return Actor{}, err
	}
	name, err := mcpserver.ParseServerName(server)
	if err != nil {
		return Actor{}, err
	}
	admitted, err := identity.ParseDigest(authority)
	if err != nil {
		return Actor{}, err
	}
	return Actor{installation: id, release: digest, authority: admitted, server: name}, nil
}

func (a Actor) InstallationID() identity.InstallationID { return a.installation }
func (a Actor) Release() identity.Digest                { return a.release }
func (a Actor) Server() mcpserver.ServerName            { return a.server }
func (a Actor) Authority() identity.Digest              { return a.authority }

func (a Actor) WithBindings(bindings *plugin.RequiredBindings) Actor {
	a.bindings = bindings
	return a
}

func (a Actor) Allows(provider plugin.Dependency, contribution plugin.ContributionID, contract jsonvalue.Value) bool {
	return a.bindings.Allows(provider, contribution, contract)
}

func (a Actor) Validate() error {
	_, err := NewActor(a.installation.String(), a.release.String(), a.authority.String(), a.server.String())
	return err
}

type actorKey struct{}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, found := ctx.Value(actorKey{}).(Actor)
	return actor, found
}

// ScopedKey separates actors while preserving a logical command across release
// and activation replacement. Release bytes are authority, not command identity.
func ScopedKey(ctx context.Context, key string) string {
	actor, found := ActorFrom(ctx)
	if key == "" || !found {
		return key
	}
	return scopedIdentity(actor.installation.String(), key)
}
