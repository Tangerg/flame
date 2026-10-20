# Public consumers

| Consumer                   | Entry                                                                           |
| -------------------------- | ------------------------------------------------------------------------------- |
| Local Go host              | `github.com/Tangerg/flame/runtime`                                              |
| Remote Go client           | `github.com/Tangerg/flame/packages/runtime-client/go` (package `runtimeclient`) |
| Go backend author          | `github.com/Tangerg/flame/packages/plugin-sdk`                                  |
| Shared wire/options        | `github.com/Tangerg/flame/runtime/protocol`                                     |
| TypeScript Runtime client  | `@flame/runtime-client`                                                         |
| Installed React component  | `@flame/component-sdk`                                                          |
| Shared controls and glyphs | `@flame/ui`, `@flame/icons/react`                                               |

Rebuild all in-repository consumers after this source/package migration. Former
Go modules, TS package names, wildcard implementation exports and source-layout
aliases are removed. Operation and stored business semantics retain their owners;
this directory migration does not convert or delete user data. Rebuild and select new plugin releases after their resource imports or contract identities change; the host does not register former package aliases.

The TypeScript client explicitly exports its main client API, `wire`, `methods`,
`validation`, `testing` and `testing/samples/*`. Ordinary unit tests and internal
implementation paths are not package APIs. `npm pack` is exercised by independent
author projects. Raw JSON sample copies are generated from Runtime-owned bytes.

## Current-source author export

Freeze a reviewed commit after selecting a fresh candidate version in dependent
manifests. `tooling/moduleproxy` uses official `x/mod/zip` rules to publish immutable
`.mod`, `.info` and `.zip` artifacts. Nested modules are archived separately;
root source is never trimmed or rewritten for authors. Candidate versions cannot
be reused for changed source, and any release tag must identify that frozen source.
`make setup` populates the task-local file proxy from the checked-out commit.

```sh
node plugins/scripts/build-author-kit.mjs \
  --plugin memory --module-path example.com/author/memory \
  --output /absolute/new/memory-author --revision HEAD
```

Use an author's own module namespace; the Flame first-party prefix overlaps the
root module and is rejected. Only that author's imports/generation identity are
rewritten. Its Go manifest declares actual generator tools and includes their
vendor dependencies. The standard SDK module retains its identity and manifest.
The author uses the same `backend/component/package` layout and explicit builder.

`--offline` resolves Go artifacts from the prepared file proxy and verified Go
cache with `GONOPROXY=none`, local toolchain and candidate-only `GONOSUMDB`. Missing
materials fail. `make check-author` additionally denies network and checkout
reads, empties build/module caches and runs test/generate/build using vendor.
Only that isolated verified-vendor phase disables checksum database requests.
The Go toolchain must already be installed; it is not vendored.

JS author projects consume real packed SDK/UI/icons archives, their own manifest
and lock, and are checked outside the workspace. Their independent npm install
requires ordinary dependency availability; this does not claim all JS transitive
or platform dependencies are included for a fully offline install.
