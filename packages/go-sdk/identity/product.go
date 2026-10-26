// Package identity owns the small shared kernel of Runtime identity grammars,
// resource envelopes, and cross-ring technical identity values. Each identity
// remains a distinct type with an explicit constructor; the package does not
// provide a generic ID or make unrelated identities interchangeable.
package identity

// ProductName is the only product brand emitted by Runtime-owned surfaces.
const ProductName = "flame"
