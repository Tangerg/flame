// Package protocol defines the binding-neutral public values of the Flame
// Runtime Protocol. HTTP and the module-root Go binding use these same requests,
// responses, events, errors, version values, and strict validators.
//
// This package deliberately contains no server method interface, context key,
// transport envelope, numeric JSON-RPC code, Host, Store, or execution handle.
// Consumers define their own narrow interfaces; the concrete Runtime is
// published by the module-root runtime package.
//
// contract/API_REFERENCE.md is the generated operation index, while the JSON
// artifacts in contract are the machine-readable method and shape authority.
// The model is Session → Run → Item: Item is the history and streaming
// primitive, and human-in-the-loop ends one Segment with an interrupt before
// the same Run resumes in another Segment.
package protocol
