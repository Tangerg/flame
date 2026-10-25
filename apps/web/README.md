# Shared Web product

This React application runs in Browser and Wails. Runtime owns durable business
semantics; Web owns inputs, view projections, navigation and presentation.
[ARCHITECTURE.md](ARCHITECTURE.md) records feature, extension and lifetime owners.
The [native host](../desktop/README.md) supplies macOS capabilities without owning
another UI application.

From the repository root:

```sh
make setup
make dev-web
npm run check --workspace @flame/web
```

Root npm workspaces and lock select all dependencies. Shared packages must be
built by `make setup`/`make generate`; do not create a package-local install or
lock. `make stage-web` builds the same production output used by native embed.

`src/app` composes product policy. `src/features` contains actual feature owners
and their formal public capabilities. `src/extensions` contains generic host,
registration and lifetime mechanisms; it never owns business extension points.
`src/platform` translates Browser/Wails effects. `src/ui` contains product
presentation composites over shared `@flame/ui` controls and tokens.

Product object menus reuse their target owner's displayed identity: view and
session titles, message authors, and complete entry paths for files. Nested
submenus keep their primitive's relationship to the controlling item instead of
receiving a separate name.

File previews keep their filename to two visible lines beside persistent navigation,
range and commands. The complete path remains selectable in the same named
scroller as source, rendered content and read/unsupported states. File requests,
line placement, renderer mode and source-return history retain their existing
owners.

MCP server rows read their own tools directly, independent of configuration
editing and server origin. Reconnect runs in the same exact-ID serial mutation
generation as other server commands; its acknowledgement only admits a
background dial, so the row shows canonical server status after the reread.

Tests stay beside their owners. Cross-language Runtime/schema/SDK tests live in
`tests/integration`; visual fixtures live in `visual`. The full check includes
compiler-resolved dependency/contract guards, lifecycle tests, resource drift,
styles, localization and the production bundle.
