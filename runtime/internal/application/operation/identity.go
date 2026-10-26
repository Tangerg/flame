package operation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/google/uuid"
)

type identityKey struct{}

type caller struct {
	id     string
	origin capability.Origin
	parent string
}

// Bind captures the delivery owner's scoped logical command. An unkeyed
// invocation is a new command; retries with a key keep exactly that identity.
func Bind(ctx context.Context, namespace, key string) context.Context {
	origin := capability.Human
	if previous, ok := ctx.Value(identityKey{}).(caller); ok {
		origin = previous.origin
	} else if _, managed := ActorFrom(ctx); managed {
		origin = capability.Automatic
	}
	return Start(ctx, origin, namespace, key)
}

// Start establishes an adapter-owned initiation. Nested Delivery calls use Bind
// to inherit this origin rather than reclassifying a callback as human input.
func Start(ctx context.Context, origin capability.Origin, namespace, key string) context.Context {
	if key == "" {
		key = uuid.NewString()
	}
	id, parent := scopedIdentity(namespace, key), Identity(ctx)
	if previous, found := ctx.Value(identityKey{}).(caller); found && previous.id == id {
		parent = previous.parent
	}
	return context.WithValue(ctx, identityKey{}, caller{id: id, origin: origin, parent: parent})
}

// Adopt projects an identity already admitted by an execution owner. In
// particular model Tool identity belongs to the durable Interaction boundary.
func Adopt(ctx context.Context, id string, origin capability.Origin, parent string) context.Context {
	return context.WithValue(ctx, identityKey{}, caller{id: id, origin: origin, parent: parent})
}

func scopedIdentity(namespace, key string) string {
	encoded := binary.BigEndian.AppendUint64(nil, uint64(len(namespace)))
	encoded = append(encoded, namespace...)
	encoded = append(encoded, key...)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func Identity(ctx context.Context) string {
	value, _ := ctx.Value(identityKey{}).(caller)
	return value.id
}

func Capture(ctx context.Context, target capability.Target) (capability.Invocation, error) {
	value, _ := ctx.Value(identityKey{}).(caller)
	invocation := capability.Invocation{ID: value.id, Origin: value.origin, Parent: value.parent, Target: target}
	return invocation, invocation.Validate()
}
