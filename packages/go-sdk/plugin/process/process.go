// Package process names what the Runtime binds for a package backend process.
package process

// The Runtime binds these for every package process; a package may also spell
// them as ${PLUGIN_ROOT} and ${PLUGIN_DATA} placeholders in its declarations.
const (
	RootVariable = "PLUGIN_ROOT"
	DataVariable = "PLUGIN_DATA"
)
