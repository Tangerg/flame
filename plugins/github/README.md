# GitHub connector

The optional `flame.github` backend reads GitHub.com issues, pull requests,
discussion, reviews, changed files, CI checks and UTF-8 repository files. It also
creates and edits issues, manages discussion and inline comments, creates PRs,
submits reviews and performs guarded direct merges. Typed operations and MCP tools
enter the same use cases. GitHub owns every upstream business fact; this backend
owns original command admission, confirmed response receipts and read projections.
It does not start Runs, construct models or compute accounting.

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
permission for issue operations, Pull requests read for PR details/files/reviews,
Checks read for check runs/annotations, Commit statuses read for external CI and
Contents read for file reads in their selected repositories. See GitHub's
[authentication documentation](https://docs.github.com/en/rest/authentication/authenticating-to-the-rest-api).

Every write requires a configured token and the user's corresponding intent.
Fine-grained tokens require Issues write for issues, Issues or Pull requests write
for discussion comments, Pull requests write for PR creation/reviews/inline comments,
and Contents write for merging. GitHub enforces repository access, actor permissions,
branch protection and review restrictions. Enabling this installation does not grant
standing approval to its model tools; Runtime retains source-qualified tool policy.

`PLUGIN_DATA/github.db` retains original command inputs and value/refusal receipts,
plus conditional-response projections keyed by the exact API request/version and
credential fingerprint. It stores neither a credential nor another upstream issue
database. Every read contacts GitHub with current authority;
`304` uses only the captured body matching that request's ETag. Credential changes
cannot borrow another scope's cached response. Authentication, transport and storage
failures never return stale cached content. The cache retains at most 64 MiB of
response data/metadata, excluding SQLite overhead; eviction affects projections only.

This release replaces the former cache-only private schema. It refuses that old
directory without converting it or creating command tables in it. Retain the old
installation and private directory separately, then use a fresh installation/data
directory with this complete release. Re-enter its token for the new executable.

## Writes and original outcomes

Use the same `github-operations` contribution, installation target and complete
generated contract. Human commands carry an idempotency key. Model tools obtain
their logical identity from Runtime metadata; arguments cannot select another key.

| Typed method                                  | MCP tool                      | Operation                                                                                            |
| --------------------------------------------- | ----------------------------- | ---------------------------------------------------------------------------------------------------- |
| `flame.github/create-issue.v1`                | `create_issue`                | Create an issue with title, optional body, labels and assignees.                                     |
| `flame.github/update-issue.v1`                | `update_issue`                | Patch supplied title/body/state/reason/labels/assignees.                                             |
| `flame.github/create-issue-comment.v1`        | `create_issue_comment`        | Add discussion to an issue or PR.                                                                    |
| `flame.github/update-issue-comment.v1`        | `update_issue_comment`        | Replace an inspected discussion comment's body.                                                      |
| `flame.github/delete-issue-comment.v1`        | `delete_issue_comment`        | Delete an inspected discussion comment.                                                              |
| `flame.github/create-pull-request.v1`         | `create_pull_request`         | Create a PR from existing branches, optionally as a draft or from a fork.                            |
| `flame.github/submit-pull-request-review.v1`  | `submit_pull_request_review`  | Create and submit an explicit review event at the inspected commit, optionally with inline comments. |
| `flame.github/merge-pull-request.v1`          | `merge_pull_request`          | Direct merge with the inspected head SHA and explicit merge method.                                  |
| `flame.github/create-pull-request-comment.v1` | `create_pull_request_comment` | Add an inline code-review comment at an inspected commit/path/line/side.                             |
| `flame.github/reply-pull-request-comment.v1`  | `reply_pull_request_comment`  | Reply to a top-level inline comment in its existing thread.                                          |
| `flame.github/update-pull-request-comment.v1` | `update_pull_request_comment` | Replace an inspected inline comment's body.                                                          |
| `flame.github/delete-pull-request-comment.v1` | `delete_pull_request_comment` | Delete an inspected inline comment.                                                                  |

Input preserves authored text. Empty update body clears it; empty `labels` or
`assignees` arrays clear those sets. Omission preserves the corresponding upstream
field. Metadata arrays are replacement sets, so inspect current issue metadata
before editing them. GitHub has no atomic revision condition for issue/comment
edits; the plugin does not invent a local revision or claim that a prior read locks
the resource. Responses expose GitHub's actual issue ID, metadata and state reason,
including label/assignee changes GitHub can ignore when the actor lacks push access.
Closing as a duplicate requires `duplicateIssueId`, the canonical issue's global
`id` returned by `get_issue`, rather than its repository-local `number`.

Review submission requires an explicit `commitId` and `APPROVE`, `REQUEST_CHANGES`
or `COMMENT` event. The latter two require a body. The single GitHub request creates
and submits that review; no pending review or local review state machine is created.
Inline ranges use `line`/`side` and paired `startLine`/`startSide`; the plugin does not
translate outdated lines to a new head. PR creation never pushes or creates commits.

Merge requires `headSha` and `mergeMethod` (`merge`, `squash`, `rebase`). GitHub checks
that SHA while merging; a changed head returns `conflict`, and a blocked merge
returns `not_allowed` or `forbidden`. A successful result requires GitHub's explicit
merged flag and merge commit SHA. Merge queues and stacked PRs require GitHub's
separate asynchronous API; this direct operation does not implement or emulate it.

Before any remote write, one short private transaction retains the exact original
JSON and command identity and invalidates cached read projections. Only the first
admission may perform HTTP I/O. No SQLite transaction spans that I/O. The confirmed
GitHub response or known refusal then commits as the original receipt. Recovery
reads that receipt without contacting GitHub or rerunning the operation, including
after backend restart, later edits or deletion. Changed method/input under the same
key returns `command_conflict`. Human and model receipts project this same record.

Transport loss, cancellation, 5xx, redirects, malformed/oversized success bodies
and local settlement failure leave the outcome unconfirmed. GitHub may already
have changed the resource. No retry, current-resource comparison, matching-comment
search or synthetic GitHub idempotency key can establish the original command's
outcome. An absent receipt never authorizes another attempt. Explicitly retiring
Runtime's retained command abandons recovery; it does not cancel or undo upstream
work. Known argument, credential, permission, rate-limit and validation refusals
remain immutable results of that original command.

Titles are bounded to 256 Unicode code points, comment/body text to 65,536 and each
encoded outbound write to one MiB. Success acknowledgements remain bounded by the
same four-MiB response policy as reads. Credentials and raw GitHub error bodies are
not copied into errors or receipts.

## Queries

Use generic `plugins.call` / `plugins call --request ...` with the contribution's
installation, release, contract and an installation target. Queries have no command
key or mutation receipt.

| Typed method                                 | MCP tool                     | Input                                                                               |
| -------------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------- |
| `flame.github/list-issues.v1`                | `list_issues`                | `repository`, `state` (`open`, `closed`, `all`), `page` (positive), `limit` (1–100) |
| `flame.github/get-issue.v1`                  | `get_issue`                  | `repository`, positive issue `number`                                               |
| `flame.github/read-file.v1`                  | `read_file`                  | `repository`, relative `path`, optional nonempty `ref`                              |
| `flame.github/list-pull-requests.v1`         | `list_pull_requests`         | Issue-list inputs, optional `base` branch and `head` (`owner:branch`)               |
| `flame.github/get-pull-request.v1`           | `get_pull_request`           | `repository`, PR `number`                                                           |
| `flame.github/list-pull-request-files.v1`    | `list_pull_request_files`    | `repository`, PR `number`, `page`, `limit`                                          |
| `flame.github/list-issue-comments.v1`        | `list_issue_comments`        | `repository`, issue/PR `number`, `page`, `limit`                                    |
| `flame.github/list-pull-request-reviews.v1`  | `list_pull_request_reviews`  | `repository`, PR `number`, `page`, `limit`                                          |
| `flame.github/list-pull-request-comments.v1` | `list_pull_request_comments` | `repository`, PR `number`, `page`, `limit`                                          |
| `flame.github/list-check-runs.v1`            | `list_check_runs`            | `repository`, commit `ref`, `page`, `limit`                                         |
| `flame.github/list-check-annotations.v1`     | `list_check_annotations`     | `repository`, `checkRunId`, `page`, `limit`                                         |
| `flame.github/list-commit-statuses.v1`       | `list_commit_statuses`       | `repository`, commit `ref`, `page`, `limit`                                         |

PR details retain the head/base SHA, draft flag, merge timestamp, mergeability and
change counts. Missing `mergeable` means GitHub has not computed an answer; it is
not a conflict verdict. A deleted fork can have no head repository. Discussion
comments, review decisions and inline comments remain distinct upstream resources.
Inline comments retain the original commit/line and reply parent; a missing current
line does not move an outdated comment onto the latest diff.

Changed files retain renames and optional patches. An absent patch does not mean
an empty diff. GitHub exposes at most 3000 PR files; `limitReached` marks this
ceiling and ends the exposed continuation without claiming completeness. Pages
are live upstream reads, not a frozen PR snapshot. Read exact head/base file
contents and checks using the SHAs from the inspected PR when revision matters.

Check runs use GitHub's `latest` filter, expose unfinished status and nullable
conclusions, and retain original diagnostic text and annotation counts. The
annotation query supplies line diagnostics using the returned check run ID.
GitHub limits ref-based checks to the most recent 1000 check suites. Commit
statuses separately cover external CI and preserve upstream reverse chronological
history; an older successful status does not supersede a newer pending/failing one.
Empty check/status pages do not establish that branch protection requirements pass.

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
[pull requests](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2026-03-10),
[reviews](https://docs.github.com/en/rest/pulls/reviews?apiVersion=2026-03-10),
[review comments](https://docs.github.com/en/rest/pulls/comments?apiVersion=2026-03-10),
[checks](https://docs.github.com/en/rest/checks/runs?apiVersion=2026-03-10),
[commit statuses](https://docs.github.com/en/rest/commits/statuses?apiVersion=2026-03-10),
[contents](https://docs.github.com/en/rest/repos/contents?apiVersion=2026-03-10) and
[conditional request](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-conditional-requests)
contracts define the external boundary. Enterprise endpoints, OAuth flows,
asynchronous merge queues and context contribution require their own demonstrated needs.

Generate with `go generate ./plugins/github/internal/contract` and verify the
changed backend with `go test ./plugins/github/backend/... -count=1` from
the repository workspace. Tests use deterministic HTTP transports and no live
GitHub credentials.
