// Package ownership owns process-local admission and cross-process ownership
// coordination for Runtime resources and recovery work.
package ownership

// Lease is one held cross-process ownership claim.
type Lease interface {
	Release()
}
