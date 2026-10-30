# Public consumers

| Consumer                   | Entry                                                                |
| -------------------------- | -------------------------------------------------------------------- |
| Generic Go plugin host     | `github.com/Tangerg/flame/qimu`                                      |
| Generic Go backend author  | `github.com/Tangerg/flame/qimu/mcp`                                  |
| Generic TypeScript backend | `@qimu/node`                                                         |
| Generic browser host       | `@qimu/browser`, `@qimu/react`                                       |
| Generic TypeScript values  | `@qimu/sdk`, `@qimu/sdk/json`                                        |
| Local Go host              | `github.com/Tangerg/flame/runtime`                                   |
| Remote Go client           | `github.com/Tangerg/flame/packages/go-sdk/client` (package `client`) |
| Go backend author          | `github.com/Tangerg/flame/packages/go-sdk/plugin`                    |
| TypeScript backend author  | `@flame/backend-sdk`                                                 |
| Memory role implementation | `github.com/Tangerg/flame/packages/go-sdk/plugin/memory`             |
| Shared wire/options        | `github.com/Tangerg/flame/packages/go-sdk/protocol`                  |
| TypeScript Runtime client  | `@flame/runtime-client`                                              |
| Installed React component  | `@flame/component-sdk`                                               |
| Shared controls and glyphs | `@flame/ui`, `@flame/icons/react`                                    |

Rebuild all in-repository consumers after this source/package migration. Former
Go modules, TS package names, wildcard implementation exports and source-layout
aliases are removed. Operation and stored business semantics retain their owners;
the Qimu cutover uses a fresh product root and `flame-qimu.db`, without converting
historical plugin state. Preserve the old deployment and its data separately.
Rebuild and select new plugin releases after their resource imports or contract identities change; the host does not register former package aliases.

The TypeScript client explicitly exports its main client API, `wire`, `methods`,
`validation`, `testing`, `testing/samples/*` and `testing/skill-pages.json`. Ordinary unit tests and internal
implementation paths are not package APIs. `npm pack` is exercised by independent
author projects. Exact JSON values are owned by `@qimu/sdk/json`; clients import it
directly. Raw JSON sample copies are generated from Runtime-owned bytes.

## Current-source author export

Freeze a reviewed commit after selecting a fresh candidate version in dependent
manifests. `tooling/moduleproxy` uses official `x/mod/zip` rules to publish immutable
`.mod`, `.info` and `.zip` artifacts. Every module is archived from its own immutable source directory; root coordination
files are not shipped inside the public SDK. Candidate versions cannot
be reused for changed source, and any release tag must identify that frozen source.
`make setup` populates the task-local file proxy from the checked-out commit.

```sh
node plugins/scripts/build-author-kit.mjs \
  --plugin memory --module-path example.com/author/memory \
  --output /absolute/new/memory-author --revision HEAD
node plugins/scripts/build-author-kit.mjs \
  --plugin usagereports --output /absolute/new/typescript-author --revision HEAD
```

Use an author's own module namespace; the Flame first-party prefix overlaps the
maintained module namespace and is rejected. Only that author's backend imports/generation identity
are rewritten. Public consumer contracts retain their publisher's namespace and
exact schemas. Its Go manifest declares actual generator tools and includes their
vendor dependencies. The standard SDK module retains its identity and manifest.
The author uses the same `backend/component/package` layout and explicit builder.
Contract-definition tools stay with the publisher. Memory author exports bind the
public `packages/go-sdk/plugin/memory` contract, including after regeneration, and
can be selected by the unchanged host consumer. Source relocation is not a new
capability identity. Build scripts convert file URLs with Node's `fileURLToPath`;
author directories may contain spaces, Unicode and URL-reserved characters.

`--offline` resolves Go artifacts from the prepared file proxy and verified Go
cache with `GONOPROXY=none`, local toolchain and candidate-only `GONOSUMDB`. Missing
materials fail. `make check-author` additionally denies external network and checkout
reads, empties build/module caches and runs test/generate/build using vendor.
Go tests may use loopback HTTP for SDK callback fixtures. A sandbox probe verifies
that local callbacks work while external connections and checkout reads stay denied;
generation, builds and managed JavaScript checks retain the complete network denial.
Only that isolated verified-vendor phase disables checksum database requests.
The Go toolchain must already be installed; it is not vendored.
TypeScript-only exports have no Go module or DTO source prerequisite. They include
the host-specific prebuilt `qimu` and `flame-sdk` contract generators, Runtime's
`pluginpack` release builder and the selected
Node test companion; their generated JavaScript ZIP remains portable. The
independent check denies Go execution through the author PATH as well as network
and checkout reads while regenerating, checking, testing and packaging that project.
The check includes a Memory author directory with spaces and Unicode, then installs
the independent release ZIP, selects its exact Memory role, invokes it and verifies
its original private command outcome after Runtime/backend restart.

JS author projects consume real packed SDK/UI/icons archives, their own manifest
and lock, and are checked outside the workspace. The initial lock projects external
selections from the frozen root lock and replaces workspace links with packed
artifacts. npm prunes unused dependencies during installation, so export works
with the tarball cache populated by a clean root `npm ci`, without relying on
registry metadata from earlier installs. `make check-author` reinstalls from the
exported lock and checks types and builds while network and checkout reads are
denied. Independent installation still requires ordinary dependency availability;
this does not claim all JS transitive or platform dependencies are included for a
fully offline install.
