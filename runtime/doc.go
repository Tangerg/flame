// Package runtime exposes Flame's typed Go operations. Open owns a complete
// in-process Runtime; Connect attaches a Client to an existing HTTP Runtime.
// Both bindings enter the same operation endpoint and expose protocol requests,
// responses, and event iterators. Operation errors support errors.Is against
// protocol sentinels and errors.As to protocol.ProblemError. Transport loss does
// not establish the outcome of a dispatched mutation.
package runtime
