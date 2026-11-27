# GenUI

GenUI presents official Google A2UI 1.0 messages through a trusted Flame catalog.
The renderer and plugin share one A2UI Web Core instance through the React and
Markdown packages' declared dependency ranges. The protocol parser, message
processor, graph validation, dynamic bindings, functions and surface model
belong to A2UI. The host renders catalog components with `@flame/ui`.
A2UI catalog schemas use Zod 3 through the `zod3` dependency.
GenUI derives the tool's request and result declarations from its Zod 4 document
schemas. A2UI parses the messages and validates their graph before acceptance.

## Discovery and presentation

`ui_catalog` with `kind: "catalog"` returns the official protocol schema and the
complete trusted catalog. Its identity is `https://flame.app/a2ui/catalog/v1`.
The catalog currently supports Text, Row, Column, List, Card, Divider, Button,
TextField, CheckBox, Tabs, and a trusted Dialog component. Dynamic functions are
restricted to the registered basic functions; external navigation is excluded.

`ui_present` accepts `{title, messages, operations}`. Messages must use version
`1.0`. Each initial `createSurface` names the trusted catalog and sets
`sendDataModel: false`. Component graphs and bindings are validated strictly by
the official processor before the result is accepted. Admission bounds message
count, operation count, JSON depth and bytes. The accepted result includes its
captured contributions within the same capacity used to read history.
No generated JavaScript, HTML, arbitrary component imports or renderer RPC is
accepted.

The package declares its exact presentation tool as an inline result. The
workbench places the generated surface in the answer and keeps the recorded
call and its arguments available in the tool disclosure.

Editable controls bind under `/draft`. Subsequent recorded `updateDataModel`
messages write named presentation fields under `/data/`. The Runtime observer
owns `/data/operations/<operation id>` and exposes it as a read-only binding for
the presentation. Business identifiers and exact quantities use
strings. A field's current native value and validation state are projected from
the A2UI model. Remote presentation messages cannot overwrite an edited draft.
Closing a trusted Dialog returns to its opener and keeps the surface's draft
values available when the same dialog is reopened.

## Captured actions

An event names a declared operation, `followup`, or `resetDraft`, and carries
explicit A2UI context. `followup` requires `{message, payload}` and submits to
the original tool interaction. `resetDraft` explicitly restores the accepted
initial draft. Rendering history performs no business query or command.

Operation discovery uses `kind: "operations"`. Index pages describe real
installation, contribution and method identities. Exact method discovery returns
its declared input and output schemas. Presentation captures that contract,
installation digest and target; discovery does not grant command permission.

Queries and calls enter the existing Runtime plugin method boundary. Commands
require the existing user review, permission and mutation journal. Their original
idempotency keys remain attached to the captured Run, Item, Session and
installation. The card reads command availability from the SDK journal and stays
locked while an original claim is active or recoverable, including across views.
Recovery reads the original receipt. The card publishes operation output when
its own attempt or recovery observes that output.

## Lifetime and scrolling

A component source retains each accepted result's MessageProcessor outside the
virtual row, keyed by its original Run and Item. Row remounts reuse that processor
and its draft. View withdrawal detaches action dispatch; source withdrawal disposes
the retained processors. Neither replay nor remount submits an operation.

The shared Legend reading list owns scrolling and end following. Its public
pinning API retains focus, composition, native selection ranges and React portal
owners. It uses public cancel, anchor and scroll intents rather than changing
`scrollTop` from a second controller.

## Breaking migration

Plugin version 3 removes the OpenUI language, private expression evaluator,
private draft store, maps, sandbox, automatic operation steps and compatibility
adapters. Older results are refused with an explicit raw-export message. Their
recorded bytes are retained by Runtime; this renderer never rewrites history.

The production CSP remains unchanged. No `unsafe-eval` or `wasm-unsafe-eval` is
added. Highlights remains deferred because it requires a conflicting Wasm policy;
recorded output keeps its ANSI parser. Code and diffs use Pierre with `shiki-js`.

Contract tests cover catalog discovery, strict admission, exact string values,
local draft preservation and old-format refusal. Browser acceptance must also
exercise the built plugin under production CSP and the Legend/A2UI combination:
upper-row reflow, a lower-row field, native selection, portals, prepend and hidden
Activity. Synthetic composition events do not establish native IME support, and
Playwright WebKit does not establish native WKWebView support.
