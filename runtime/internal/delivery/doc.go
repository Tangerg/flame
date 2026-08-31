// Package delivery owns Runtime's binding-neutral Endpoint, operation catalog,
// protocol handlers, and presentation. Every binding enters through Endpoint so
// validation, capability gating, idempotency, lifecycle, error projection, and
// event filtering have one implementation.
package delivery
