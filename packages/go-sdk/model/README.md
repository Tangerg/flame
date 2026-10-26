# Model selection

This public value owns the provider/model/reasoning identity shared by Runtime and
independent feature authors. Construct it with `New` or `NewWithReasoningEffort`.
The zero value requests the owning use case's default; a configured value names
provider and model together. Provider inference is not supported. `ValidateExact`
requires a configured identity when the calling operation needs one.

`Patch` changes that identity atomically. Provider and model must be supplied
together. Changing the pair without an explicit effort resets reasoning to the
new model's provider default; an effort-only edit retains the current pair.

JSON and generated schema share the closed `provider`, `model`, `reasoningEffort`
field set. All three strings are required, including empty strings for default
selection. Failed decoding does not mutate a previous value. The public plugin
model-selection policy transports its own explicit query projection; Runtime
still admits the chosen identity before provider construction.

`SelectionContext` projects the actual interaction, utility or independent use.
Only utility and independent calls already own a bounded input/output envelope at
selection time. `SelectionLimits` preserves its input unit: text bytes for utility
work and complete encoded-request bytes for independent calls. These values inform
routing; capacity admission, envelope enforcement and pricing retain their core
owners. Interaction does not invent an envelope from its current input size.

[`model/catalog`](catalog/README.md) supplies shared admission against the released
Scope model catalog. Host role configuration, physical endpoint evidence and
execution token-limit policy retain their existing owners. Former private
Selection constructors/readers are removed; source consumers migrate directly.
Moving the schema owner changes Go-derived reference identities. Regenerate and
rebuild Schedules contracts, Go/TS consumers and component bytes together; no former
contract alias or reader is retained.
