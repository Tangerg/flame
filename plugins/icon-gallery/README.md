# Icon gallery

The optional static view displays Flame's canonical icon catalog and its 24 px SVG
exports. It is generated from the existing `@flame/icons` package, which remains the
sole owner of geometry, optical masters and discovery metadata. Do not edit the
generated HTML or add a second icon catalog to this package.

Build the local icon package and install the frontend's existing dependencies first,
then run from the repository root:

```sh
node plugins/icon-gallery/build.mjs
node plugins/icon-gallery/build.mjs --check
```

The frontend check verifies the generated resource is current. The admitted release
is self-contained and needs no network, clipboard, file, Runtime query or mutation
authority. Search only filters immutable cards within that page. Install, approve and
enable the package explicitly to expose its workspace view. The former compiled
gallery and advanced settings showcase are removed.
