// Package identity owns the CLI domain's admission rules for exact foreign
// Runtime identities and model selection. It does not create a second identity
// representation, infer providers, normalize opaque values, or own Runtime
// lifecycle state.
package identity

// ValidateSession admits an exact opaque Runtime session identity received from
// the Runtime, a command, or durable CLI state without normalizing it.
func ValidateSession(value string) error {
	return validateOpaque("session id", value, MaximumResourceCharacters)
}
