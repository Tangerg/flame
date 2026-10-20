# Local Runtime handoff

This independent Go module owns process deployment roots, strict token handoff and
companion `plugins/` lookup. It is
shared by Runtime, CLI and the native Desktop host without importing their
business implementation.

`FLAME_HOME` selects an absolute product root. Empty selects the user's normal
Flame root; explicit values retain meaningful whitespace. Token files are
private process handoff, never a printable URL preference. Each delivered
executable carries its companion plugins directory as required by this module's
lookup contract.

Develop through root `go.work`; root setup/check discovers this member.
Independent source distribution selects its immutable candidate module version
before freezing source and packages it separately from the root module.
