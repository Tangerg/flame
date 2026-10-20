package identity

// MaximumCursorCharacters is the largest opaque pagination cursor Flame will accept
// or emit. Pagination cursors use URL-safe Base64 and are therefore ASCII, so a
// character is exactly one encoded byte at every boundary.
const MaximumCursorCharacters = 64 * 1024
