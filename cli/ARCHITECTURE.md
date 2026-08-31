# Flame CLI architecture

Flame CLI is a consumer of the public in-process Runtime binding. It provides one-shot Cobra commands and an interactive Oolong terminal client without creating a second product backend.

## Ownership

Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, provider selection, persistence, compaction, and recovery.

CLI owns only command and presentation concerns:

- process signals, streams, diagnostics, and exit status;
- typed CLI preferences and command input;
- terminal focus, selection, layout, navigation, overlays, and rendering;
- drafts, attachments, prompt history, queue intent, stash, and replay intent;
- projections used to present Runtime facts.

A Runtime event may update a preview, but completed Items and authoritative snapshots win after reconnect, gaps, and cold recovery.

## Dependency direction

The CLI uses four responsibility rings:

| Ring | Responsibility |
| --- | --- |
| Domain | Pure CLI-owned values and local aggregate invariants |
| Application | CLI workflows and consumer-owned ports |
| Adapter | Translation to Runtime and filesystem boundaries |
| Delivery | Cobra routing, terminal interaction, and rendering |

Application depends on Domain. Adapters and Delivery depend inward on the consumer contracts they satisfy. Cobra and Viper remain in command delivery; Oolong remains in terminal delivery; public Runtime and Protocol types remain in `adapter/runtimebinding`.

Domain contains no I/O interfaces and no `context.Context`. A narrow port is declared beside the Application or Delivery behavior that consumes it. Composition supplies the concrete implementation.

## Runtime path

The production process opens at most one concrete Runtime, fans its binding adapter into consumer-owned ports, and closes the Runtime once. There is no environment-selected fake, loopback HTTP client, service locator, or alternate product implementation.

`runtimebinding.Connection` owns binding lifecycle, capability negotiation, exact protocol translation, and safe error classification. It does not own product state. The adapter preserves exact provider/model identity and never stores credentials in CLI state, history, frames, errors, or logs.

## Commands

Each command tree is built by a factory. A command declares syntax and flags, converts input to a typed request, calls one CLI use case with `cmd.Context()`, and writes through Cobra streams. Runtime failures return through `RunE`; the process boundary chooses the exit status once.

Viper is only a configuration input. It resolves defaults, files, environment, and flags into typed CLI preferences before use-case execution. Business logic does not import Cobra or Viper.

## Terminal

The terminal package owns an explicit UI state tree and cohesive feature controllers. Rendering is a projection of state and terminal dimensions; it does not repair Runtime facts or start hidden workflows.

Oolong owns terminal mode, input decoding, cell measurement, and low-level editing. Flame owns product interaction, including focus, keymaps, mouse press/release matching, overlays, composer behavior, attention signals, and stable stream presentation.

Long-lived terminal features own their cancellation and settlement locally. The application root coordinates them but does not mirror every feature field or become a general service bag.

## Local authoring

Workbench persistence contains only CLI-authored facts. Records use one strict current shape and fail closed on unknown, malformed, oversized, truncated, or trailing content. Queue and replay are CLI aggregates with explicit identities and legal transitions; terminal code commands them instead of mutating slices and flags independently.

Attachments are local path references. Dispatch reopens the current file through the filesystem adapter and converts it to Runtime content under explicit size and encoding limits.

## Package shape

A package must own a coherent CLI vocabulary, local aggregate, workflow lifecycle, external translation, or terminal mechanism. Related behavior stays in responsibility-named files inside one package. Context namespace directories exist only for several peer packages and contain no facade Go files.

Do not create packages for individual actions, interfaces, or DTOs. Do not create broad `service`, `manager`, `backend`, `common`, or `helpers` packages.

## Verification

Use fresh-root in-memory tests for command syntax and streams, Application tests for workflow ordering, deterministic terminal state/render tests for interaction behavior, and a real PTY only for terminal mode, input decoding, resize, focus, or restoration. Architecture checks enforce dependency direction and framework isolation without freezing private fields, filenames, or package inventories.
