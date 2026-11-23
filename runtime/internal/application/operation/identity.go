package operation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	framecall "github.com/Tangerg/frame/call"
	"github.com/google/uuid"
)

type identityKey struct{}

type caller struct {
	id     string
	origin framecall.Origin
	parent string
}

func Bind(ctx context.Context, namespace, key string) context.Context {
	origin := extension.Human
	if previous, ok := ctx.Value(identityKey{}).(caller); ok {
		origin = previous.origin
	} else if _, managed := ActorFrom(ctx); managed {
		origin = extension.Automatic
	}
	return Start(ctx, origin, namespace, key)
}

func Start(ctx context.Context, origin framecall.Origin, namespace, key string) context.Context {
	if key == "" {
		key = uuid.NewString()
	}
	id, parent := scopedIdentity(namespace, key), Identity(ctx)
	if previous, found := ctx.Value(identityKey{}).(caller); found && previous.id == id {
		parent = previous.parent
	}
	return context.WithValue(ctx, identityKey{}, caller{id: id, origin: origin, parent: parent})
}

func Adopt(ctx context.Context, id string, origin framecall.Origin, parent string) context.Context {
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

func Capture(ctx context.Context, target framecall.Target) (framecall.Invocation, error) {
	value, _ := ctx.Value(identityKey{}).(caller)
	invocation := framecall.Invocation{ID: value.id, Origin: value.origin, Parent: value.parent, Target: target}
	return invocation, extension.ValidateInvocation(invocation)
}
