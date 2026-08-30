// Package cursorresource owns the transport-neutral resource envelope shared by
// pagination authorities and wire adapters. It contains no query, transport, or
// product semantics; those layers project this one ceiling into their own
// validation contracts.
package cursorresource

// MaximumCharacters is the largest opaque pagination cursor Flame will accept
// or emit. Pagination cursors use URL-safe Base64 and are therefore ASCII, so a
// character is exactly one encoded byte at every boundary.
const MaximumCharacters = 64 * 1024
