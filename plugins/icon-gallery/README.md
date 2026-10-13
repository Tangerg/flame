# Icon gallery

The optional workspace component displays Flame's canonical icon catalog and its
24 px React drawings. `@flame/icons` remains the sole owner of geometry, optical
masters and discovery metadata. The public SDK builds ordinary prebuilt ESM;
React, JSX and UI controls use the workbench's shared module instances. Icon code
and metadata are bundled private dependencies, with no second catalog or HTML guest.

Build the local icon package and UI package, install the portable toolchain, then
run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

Generated ESM is tracked for installation; edit the source and rebuild it. The
component has no callable contribution or backend process. Search filters immutable
package content through local React state. Install and enable the package explicitly
to expose its workspace view. Runtime admits its exact release resources through
the same subscription-owned module loading path as other installed components.
Appearance comes from the shared UI owner. Disabling the package withdraws its
registration; changing an evaluated release requires the usual window reload.
