package operation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/google/uuid"
)

type identityKey struct{}

// Bind captures the delivery owner's scoped logical command. An unkeyed
// invocation is a new command; retries with a key keep exactly that identity.
func Bind(ctx context.Context, namespace, key string) context.Context {
	if key == "" {
		key = uuid.NewString()
	}
	return context.WithValue(ctx, identityKey{}, scopedIdentity(namespace, key))
}

func scopedIdentity(namespace, key string) string {
	encoded := binary.BigEndian.AppendUint64(nil, uint64(len(namespace)))
	encoded = append(encoded, namespace...)
	encoded = append(encoded, key...)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func Identity(ctx context.Context) string {
	id, _ := ctx.Value(identityKey{}).(string)
	return id
}
