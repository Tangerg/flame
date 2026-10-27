# Web research

The installed backend owns Jina Reader fetching, Tavily search and a private page
snapshot archive. Typed `flame.web.v1` operations and the `web_search`, `web_fetch`
and `web_browse` MCP tools enter the same use cases. Provider clients come from
released Scope; credentials are optional private installation inputs `jina-key`
and `tavily-key`. An unset input explicitly refuses that provider's operation.
Malformed configured values refuse backend startup. Parent process credentials
do not become installation inputs.

Search accepts Scope's query, result count, domain filters and recency vocabulary.
Tavily refuses unsupported hourly recency. Queries are bounded to 8192 bytes;
encoded search results are bounded to 128 KiB and the requested result count.
Every search contacts the configured provider. Search responses are not retained
in the archive.

Fetch accepts an absolute HTTP(S) URL without embedded credentials and Scope's
markdown, HTML or text format. Every fetch contacts Jina Reader, admits its actual
response format and captures up to two MiB of UTF-8 content. The response contains
an immutable content-qualified `snapshotId`, source URL, total Unicode codepoints
and a first page of up to 8192 codepoints. The source URL records the requested
resource, rather than inventing the reader's final redirect target.

Use `browse.v1` with that ID, an explicit `offset` and a `limit` from 1 to 8192 to
continue. `nextOffset` is absent at the end. Browse reads exactly that captured
version without provider I/O, even when a later fetch observes changed content.
It is a snapshot reader; it does not run JavaScript or control a graphical browser.

Private `web.db` retains the newest 32 distinct captures within a 64 MiB logical
JSON-content budget. Repeated identical captures share an ID; insertion order is
not refreshed by reads. Evicted snapshots return `snapshot_not_found`, never a
silent refetch. Stored IDs bind URL, format and exact content; corrupt records
refuse instead of being overwritten or substituted. The archive stores neither
API keys nor a mutable "latest page" pointer.

HTTP provider responses are bounded to eight MiB before Scope decoding, requests
have a 30-second deadline and credential-bearing redirects remain on their
original origin. Unexpected read failures retain their diagnostic cause while
the model receives a definite failed read. Cancellation retains core control
semantics. The plugin constructs SaaS clients; model provider construction,
execution, waiting recovery and accounting retain their core owners.

Migration: remove `online.jinaApiKey` / `online.tavilyApiKey` from Runtime YAML and
configure this installed package's secret inputs. The former
`FLAME_JINA_API_KEY` / `FLAME_TAVILY_API_KEY` overlays no longer create core tools
or seed plugin values. Current tools have installed MCP identities. Historical
built-in web references and normalized result schemas remain readable; there is
no old executable, alias or automatic retargeting. Finish waiting work captured
by an older build with that original build. The method-selectable HTTP tool keeps
its separately owned host allowlist.

Regenerate contracts with `make generate`.
Provider contracts: [Scope web](https://github.com/Tangerg/scope/tree/main/tools/web),
[Jina Reader](https://jina.ai/reader/) and
[Tavily Search](https://docs.tavily.com/documentation/api-reference/endpoint/search).

The maintainer-only `contractgen` command owns the capability descriptor. It
derives schemas from the domain and pure wire declarations. `make generate`
updates `contracts/contract.generated.go` from that owner before refreshing package
projections. Backend bindings consume the exact published descriptor through Qimu.
An independent author module's `go generate` refreshes package projections from
that descriptor; it does not derive another contract from relocated implementation
types.
