# GitHub connector

The optional `flame.github` backend reads GitHub.com issues, pull requests and
UTF-8 repository files. Typed operations and MCP tools call the same query owner.
It never writes to GitHub, starts a Run, constructs a model, or computes accounting.

Build/install this package through the ordinary
[`first-party distribution`](../README.md). Its `github-operations` contribution
exports `flame.github.v1`; generated contracts and TypeScript bindings live in
`io.github.tangerg.flame/contracts`.

## Credentials and private data

Configure the optional secret input `token` through `plugins.configure` using the
installation's exact selected digest. Runtime's existing private installation
input store is the sole credential owner. It redacts inspection and delivers the
credential only as `GITHUB_TOKEN` to the selected executable. The backend retains
an immutable process-lifetime value and does not copy the token into its database,
query inputs, responses or portable package. Changing/clearing it uses the existing
quiescence and backend replacement lifecycle. Stdio release changes require entering
the credential again because the recipient executable changed.

An unset input permits unauthenticated public reads. A configured empty/invalid
token refuses backend startup; a rejected token returns `unauthorized`. Neither
path silently switches to public access. Fine-grained tokens need Issues read
permission for issue operations and Contents read permission for file reads in
their selected repositories. See GitHub's
[authentication documentation](https://docs.github.com/en/rest/authentication/authenticating-to-the-rest-api).

`PLUGIN_DATA/github.db` owns conditional-response projections, keyed by the exact
API request/version and credential fingerprint. It stores neither a credential nor
another upstream issue database. Every read contacts GitHub with current authority;
`304` uses only the captured body matching that request's ETag. Credential changes
cannot borrow another scope's cached response. Authentication, transport and storage
failures never return stale cached content. The cache retains at most 64 MiB of
response data/metadata, excluding SQLite overhead; eviction affects projections only.

## Queries

Use generic `plugins.call` / `plugins call --request ...` with the contribution's
installation, release, contract and an installation target. Queries have no command
key or mutation receipt.

| Typed method                  | MCP tool      | Input                                                                               |
| ----------------------------- | ------------- | ----------------------------------------------------------------------------------- |
| `flame.github/list-issues.v1` | `list_issues` | `repository`, `state` (`open`, `closed`, `all`), `page` (positive), `limit` (1–100) |
| `flame.github/get-issue.v1`   | `get_issue`   | `repository`, positive issue `number`                                               |
| `flame.github/read-file.v1`   | `read_file`   | `repository`, relative `path`, optional nonempty `ref`                              |

`repository` is `owner/name`. Issues include a `pullRequest` discriminator because
GitHub's issue endpoints also return pull requests. Listing projects the next page
number from GitHub's Link header; later requests stay bound to the caller's
repository and filters. No link, redirect or download URL can choose an HTTP target.
Files preserve their returned path, SHA, URL and UTF-8 text. Directories, other
resource kinds, binary content and unsupported encodings are explicit refusals.
The local file ceiling is one MiB; a representation unavailable from GitHub's
Contents API is refused rather than fetched from another endpoint.

The client uses REST API `2026-03-10`, a 30-second request deadline, four MiB
response admission and a fixed HTTPS API origin. It refuses redirects and does not
retry HTTP error responses. Rate limits, forbidden/not-found resources and invalid
queries remain explicit failures. Malformed upstream data stays a protocol failure;
it cannot become an empty successful page.

GitHub's [issues](https://docs.github.com/en/rest/issues/issues?apiVersion=2026-03-10),
[contents](https://docs.github.com/en/rest/repos/contents?apiVersion=2026-03-10) and
[conditional request](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-conditional-requests)
contracts define the external boundary. Enterprise endpoints, OAuth flows, writes
and context contribution require their own demonstrated needs.

Generate with `go generate ./plugins/github/internal/contract` and verify the
changed backend with `go test ./plugins/github/backend/... -count=1` from
the repository workspace. Tests use deterministic HTTP transports and no live
GitHub credentials.
