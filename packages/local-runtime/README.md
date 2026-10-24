# Local Runtime handoff

This independent Go module owns the default local endpoint, process deployment
roots, strict token handoff and companion `plugins/` lookup. It is
shared by Runtime, CLI and the native Desktop host without importing their
business implementation.

`node-release.json` owns the exact managed Node release, target artifacts,
checksums and launch arguments. `NodeForExecutable` resolves it beneath the
host's sibling `runtimes/node/<os>-<arch>/` directory. `Node.Verify` checks the
selected executable before plugin evaluation. There is no PATH, data-root,
workspace or download fallback. Missing companions leave only affected managed
activations unavailable. Native and UI-only plugins do not require Node.

Product assembly stages the complete upstream distribution, including notices,
through `tooling/node-companion.mjs`. Build preparation may acquire the pinned
archive; tests and runtime activation never do. Replacing a companion requires
a quiescent distribution. Retain old deployment/data together for unfinished
work; its engine identity is distinct from a business idempotency key.

`DefaultListenAddress` owns the default listener; `DefaultOrigin` derives its
HTTP origin for CLI and Desktop. Explicit endpoint configuration remains owned
by each consuming process.

`FLAME_HOME` selects an absolute product root. Empty selects the user's normal
Flame root; explicit values retain meaningful whitespace. Token files are
private process handoff, never a printable URL preference. Each delivered
executable carries its companion plugins directory as required by this module's
lookup contract.

Develop through root `go.work`; root setup/check discovers this member.
Independent source distribution selects its immutable candidate module version
before freezing source and packages it separately from the root module.
