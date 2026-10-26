# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.github/get-issue.v1` | query | installation | Read one GitHub issue or pull request by its repository and issue number. |
| `flame.github/get-pull-request.v1` | query | installation | Read a pull request's head/base SHAs, draft/merge state and change counts. A missing mergeable value means GitHub has not computed it. Use the head SHA for subsequent commit checks. |
| `flame.github/list-check-annotations.v1` | query | installation | Read one page of line-level diagnostics for a check run ID returned by list_check_runs. Preserves paths, ranges, severity and original diagnostic text. |
| `flame.github/list-check-runs.v1` | query | installation | Read one page of latest check runs with upstream status, conclusion and diagnostic text. GitHub limits this endpoint to the most recent 1000 check suites. Use the exact PR head SHA, list_check_annotations for line diagnostics, and list_commit_statuses for external CI. |
| `flame.github/list-commit-statuses.v1` | query | installation | Read one page of commit statuses, including external CI that is not represented by check runs. Use an exact PR head SHA when assessing a revision. |
| `flame.github/list-issue-comments.v1` | query | installation | Read one page of discussion comments on an issue or pull request. Inline code review comments are returned by list_pull_request_comments. |
| `flame.github/list-issues.v1` | query | installation | Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub. |
| `flame.github/list-pull-request-comments.v1` | query | installation | Read one page of inline PR review comments with diff context, commit identity, reply parent and available line positions. Missing current positions can indicate outdated comments. |
| `flame.github/list-pull-request-files.v1` | query | installation | Read one live page of PR file changes and available patches. An absent patch is not an empty diff. GitHub exposes at most 3000 files; limitReached means the list may be incomplete. Read repository files at an exact PR head/base SHA when needed. |
| `flame.github/list-pull-request-reviews.v1` | query | installation | Read one page of PR reviews with their upstream state, commit and submitted time, including pending and dismissed reviews. |
| `flame.github/list-pull-requests.v1` | query | installation | List one page of pull requests with head/base refs and SHAs, optionally filtered by base or owner:branch head. |
| `flame.github/read-file.v1` | query | installation | Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused. |

Capability `flame.github.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
