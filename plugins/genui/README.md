# Generated interfaces

GenUI is an optional managed Node backend and an installed Tool-result component.
It generates interactive documents, forms, calculators, data explorers, diagrams
and isolated HTML demonstrations inside the existing conversation. It does not
start a separate model conversation or create a UI database.

`genui_ui_catalog` publishes the expression vocabulary and, when requested, the
currently available installed callable contracts. `genui_ui_present` admits one
complete JSON document and captures its actual operation bindings. Runtime's
existing Tool/Item history retains the accepted document as structured content;
plain text is a derived projection. The installed component reads that successful
captured result, never the original unaccepted tool arguments.

The MCP schemas use Qimu's portable structural profile. Authored string rules
appear in their descriptions and remain enforced by the original GenUI parser,
alongside cross-field rules, before any document or operation binding is accepted.
Invalid identifiers, SVG values and other authored strings remain business refusals.

## Ownership

| Fact                                                                | Owner                                                             | Other representations                                              |
| ------------------------------------------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------ |
| Accepted document, bounds, references and expression meaning        | Immutable `Presentation` and its component/expression definitions | MCP schemas, text, OpenUI source and renderer props                |
| Local input, selection, draft and calculator values                 | The OpenUI Renderer store                                         | Controls, expressions and form validation read the same values     |
| Business data and a business command's outcome                      | The plugin providing the captured callable contract               | GenUI displays query results and command receipts                  |
| Original command identity, captured route and unresolved acceptance | The existing Runtime client mutation journal and Runtime receipt  | GenUI keeps a recovery handle, never another receipt store         |
| Session, Run, input acceptance and conversation history             | Runtime and the shared Agent input path                           | `CapturedToolInteraction` supplies the original source and receipt |
| Map viewport and tile delivery                                      | The map renderer and its tile resource view                       | Marker selection remains in the OpenUI store                       |
| HTML demonstration state                                            | The isolated frame                                                | No privileged state or executable bridge is shared with Flame      |

Operation output has a separate, read-only expression namespace. Model documents
cannot bind an editable field to a query result or use a `set` step to change a
business result. A local draft is not saved business data. Remounting restores
accepted initial values; ordinary backend reads obtain current business data.
There is no competing localStorage history, React form-state mirror, document
revision protocol, or synchronization winner rule.

## Interface vocabulary

The single component definition catalog supplies both authored and evaluated
props. Layouts nest recursively. Ordinary values can use a closed expression
instead of a literal; authority-bearing field and action identifiers remain
literal references checked at admission.

- Layout: stack, responsive grid, card, section, tabs, accordion, disclosure,
  carousel, popover, conditional content and bounded repeated content.
- Reading: text, Markdown, code with copying, badges, callouts, links, images,
  exact string metrics, lists, progress and dividers.
- Forms: text, number and email drafts; multiline input; single and multiple
  selection; radio groups; checkbox; switch; slider; date input; validation,
  submission and reset. Number drafts remain strings until explicitly converted.
- Data: searchable, sortable, pageable and selectable tables; bar, line, area,
  pie, donut and scatter charts with accessible underlying data.
- Explanations: mathematical notation, local countdowns, flashcards, SVG groups
  and drawing primitives, and sandboxed HTML/JavaScript for canvas, simulations
  and games.
- Geography and media: a pannable, zoomable map with explicit marker data,
  selection and supplied route geometry; HTTPS images; privacy-enhanced YouTube
  embeds. Data acquisition remains an ordinary agent tool or installed query.

Controls use public `@flame/ui` components and the active host locale and tokens.
Horizontal report tabs select with Left/Right, Home and End. They leave Up/Down
to the browser, while radio choices retain both arrow axes.
Popover uses the shared non-modal portal and collision positioning, so a clipped
report does not clip its detail controls. Nested Select and Popover dismissal
use Base UI's existing layer boundary. The declared open field and all drafts
remain in the OpenUI store. Closing removes popup content through the primitive's
ordinary lifetime; hidden views retire open descendants without reopening over
later authoring. Available-height scrolling keeps larger popup content reachable.
Progress uses the same themed ProgressBar as application activity. Its complete
label and localized percentage remain visible above the track, including zero,
completion and a supplied maximum other than 100. The admitted values and local
expressions retain their existing meaning; no simulated execution or new state
owner is introduced.
Flashcard face controls name the next reading action. They retain ordinary
button semantics as their labels change between showing the answer and question;
the current face remains expressed by the existing face label and body.
Local flip state stays in the same OpenUI field.
Native chart geometry uses labels, patterns and a data table rather than color
alone. Markdown uses the established renderer with raw HTML disabled. KaTeX
produces MathML with trusted commands disabled. Map tiles use OpenStreetMap with
attribution; tile errors remain visible and can be explicitly retried. A supplied
route is displayed as supplied geometry, not represented as newly computed road
routing. Image search, geocoding, routing and remote business facts belong to
existing tools or provider plugins, rather than hidden GenUI service calls.

Image and Markdown-image thumbnails open their original source in the shared
image reader. The authored alternative text and caption retain their meaning;
local zoom and panning do not advance OpenUI fields or operation state. Resource
loading, failure and explicit retry keep their existing owner. Hiding the report
or replacing an image source ends its open reading capture.
Explicit retry hands its own focus to the surviving, named image container
before replacing the action. Loading, decoding and failure do not reclaim later
editing focus. Independent images and Markdown images share that same resource
boundary, without adding another natural Tab stop.
Markdown's tag renderers retain their component identities through independent
local updates. A private translation context supplies current labels through
the Markdown renderer's fixed DOM/AST prop contract, so images, source readers,
links and tables are not replaced to obtain translated copy. Decoding failures,
open previews and native reading positions keep their existing owners. Changing
an image's actual source still ends its old preview.

## Local computation

The document declares `state`, `nodes`, `actions` and `operations`. Expressions
reference local state, operation projections and bounded loop variables. They
can construct objects and arrays, map/filter/group records, and call the pure
functions described by the catalog. Arithmetic, comparisons, formatting,
collection transformations and statistics run locally without a model request.

The compiler lowers expressions to registered OpenUI functions through the
public serializer. It never concatenates authored operators into executable
source and never uses `eval` or `Function`. Registered functions, their schemas
and the catalog descriptions derive from the same definitions. Numeric
projections have finite bounds; identifiers and authoritative exact quantities
should remain strings. Invalid, non-finite or out-of-budget results are explicit
errors, not silently truncated output.

Model-authored OpenUI source is not an accepted input. OpenUI deliberately
recovers partial syntax; it cannot establish final-document admission. The plugin
validates the complete JSON value, cross-references, depth and aggregate budgets
before producing OpenUI source. There is one accepted format and no permissive
historical decoder. The component displays a completed document; it does not
execute an incomplete model response.

For example, this document creates a local circle-area calculator and a button
that asks the original conversation to explain the selected radius:

```json
{
  "title": "Circle lab",
  "state": [{ "id": "radius", "value": 2 }],
  "nodes": [
    {
      "id": "radiusControl",
      "type": "Slider",
      "props": {
        "field": "radius",
        "label": "Radius",
        "min": 1,
        "max": 10,
        "step": 1
      }
    },
    {
      "id": "area",
      "type": "Metric",
      "props": {
        "label": "Approximate area",
        "value": {
          "call": "multiply",
          "args": [
            { "literal": 3.141592653589793 },
            {
              "call": "power",
              "args": [{ "state": "radius" }, { "literal": 2 }]
            }
          ]
        }
      }
    },
    {
      "id": "explain",
      "type": "Button",
      "props": { "label": "Explain this result", "action": "explain" }
    }
  ],
  "actions": [
    {
      "id": "explain",
      "steps": [
        {
          "type": "followup",
          "message": {
            "call": "concat",
            "args": [
              {
                "array": [
                  { "literal": "Explain the area of a circle with radius " },
                  { "call": "string", "args": [{ "state": "radius" }] }
                ]
              }
            ]
          }
        }
      ]
    }
  ],
  "operations": []
}
```

## Query, command and conversation actions

A named action has ordered steps: change local state, reset declared fields,
execute an operation, submit a followup, copy text or open a permitted link. The
application controller awaits each effect. Invalid forms do not dispatch their
action. A pending action blocks duplicate submission. Cancellation or a known
failure stops the remaining steps and is shown explicitly.

An invalid form submission focuses its first available invalid control in the
current form and visible view. Field-specific errors describe their input,
selection trigger or choice group rather than broadcasting a live update on
every edit. The field views project the document owner's validation; they do not
admit or reject execution. Drafts, hidden-view validation and action dispatch
retain their existing owners.

An operation request supplies its local ID, installation, contribution, method,
optional resource target and an input expression. The model cannot supply the
admitted contract, release digest or operation kind. The backend resolves these
from the real enabled and available selected installation. It derives
installation targets; other scopes require explicit matching targets. The
accepted result contains the captured binding. A later release selection does
not silently change this old document's business route.

Queries use public `plugins.call` or `plugins.queryStream`. Only a real query can
be marked automatic. Commands require a user review of the actual installed
method, target and exact JSON input, then use public `plugins.command` or
`plugins.stream`. The original provider still validates inputs, owns its
business transaction and returns the original receipt. There is no generic Core
method dispatcher, raw MCP call bypass or model-authored authorization flag.

An operation expression can read `status`, `data`, the latest streaming `chunk`,
or `error`. Streaming chunks are a transient progress projection; GenUI does not
archive a second unbounded event history. The final result replaces progress.
Query output and command output are not added to a parallel conversation store.
A later ordinary followup can explicitly carry relevant local input to the model.

Unconfirmed commands retain their original intent and input in the existing
journal. Recovery uses the original key and frozen route. Intent recognition is
a read-only journal operation, so another document's command with the same
method is not mistaken for this document's action. The stable intent uses the
captured source Run/Item and action/operation IDs; upgrading the renderer does
not create a new intent. A remounted view can recover its original command.
Observation failure is not completion, and does not unlock a replacement write.

Followups use the host-provided `CapturedToolInteraction`. It captures the source
Run and Item, resolves their Session only on submission, and enters the same
send/steer, acceptance and recovery path used by ordinary Agent input. Navigation
elsewhere cannot retarget the action. `accepted` means input acceptance, not that
the ensuing agent task has finished. `unknown` keeps the original recovery
closure; `retired` is an explicit terminal outcome. Exact JSON text and source
identifiers travel in the ordinary user input; Runtime's history model is not
extended just to index a UI action.

## HTML sandbox

`HtmlView` is for self-contained HTML, inline JavaScript, SVG and canvas programs.
It runs in an opaque, scripts-only inner frame. A trusted opaque outer frame
applies a restrictive CSP to the inner frame's navigation as well as resources.
Both frames omit same-origin access, forms, popups, top navigation, downloads and
privileged browser features. No Flame context, cookies, token, tool dispatcher or
postMessage command bridge is supplied. Inline styles and local data/blob media
are supported; external scripts, frames and network requests are blocked.

The extra outer frame is deliberate: a CSP inside an authored page alone does
not constrain that page's own navigation. Real browser tests exercise local
execution, parent access, requests, navigation and wrapper escaping. Browser
isolation is not a hard CPU or memory quota for arbitrary JavaScript. Large
programs remain subject to browser resource behavior and the authored document's
size limit. Structured components should be preferred when they express the UI.

## Dependencies and reference projects

The plugin uses the public OpenUI 1.0.1 serializer, evaluator and React renderer;
it does not import private contexts or add its own parser. The interactive
component ideas are informed by
[Open Intelligent UI](https://github.com/thesysdev/open-intelligent-ui) and
[OpenUI](https://github.com/thesysdev/openui). Their model gateway, localStorage
thread history, private design system and application routing are not additional
Flame owners. Public OpenAI Visualizations, ChatKit actions and plugin UI guidance
inform the separation of local exploration, explicit effects and server-owned
business data. This is a Flame plugin contract, not an implementation of the
OpenAI `window.openai` compatibility API.

Private dependencies are bundled. React, ReactDOM, the component SDK, Flame UI
and the public Runtime client modules remain supplied by the host. Sharing the
Runtime client preserves exact-number and error identities across that boundary.
Markdown, MathML and maps use established public
libraries; their notices ship in the portable package. New dependencies belong
to the root workspace manifest and lock, not a plugin-local install graph.

## Build, verification and breaking change

Use the toolchain in `DEVELOPMENT.md` and install at the repository root:

```sh
npm run build --workspace @flame/plugins
npm run check --workspace @flame/plugins
npm test --workspace @flame/web -- visual/genui
node plugins/scripts/build-packages.mjs --plugin genui --output /absolute/new/packages --archives /absolute/new/releases
```

The ordinary builder packages the complete managed backend graph, declared
component resources and notices. Installation does not compile code or download
runtime dependencies. The author exporter derives its independent dependency
graph from actual imports, the frozen workspace manifest/lock and SDK peers.

Version 2 replaces the earlier read-only `blocks` contract with this interactive
contract. There is no old-format adapter. An earlier v1 captured report is not
rewritten or treated as a valid v2 document; retain its original release when
that renderer is needed, or generate a new interface. The final patch contains
the complete implementation against the stated upstream base.

Verification covers strict owner admission, references and budgets; the actual
MCP backend; real OpenUI/store updates; form and data interactions; exact input
and command recovery through the Runtime client; captured Session submission;
portable packaging and independent author export; and browser rendering and
sandbox isolation. Native Wails qualification still requires macOS and the
repository's `make check-native` gate.
