# Icon gallery

The optional static view displays Flame's canonical icon catalog and its 24 px SVG
exports. It is generated from the existing `@flame/icons` package, which remains the
sole owner of geometry, optical masters and discovery metadata. Do not edit the
generated HTML or add a second icon catalog to this package.

Build the local icon package and build `ui` and install the portable page toolchain first,
then run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

The frontend check verifies the generated resource is current. The admitted release
is self-contained and needs no network, clipboard, file, Runtime query or mutation
authority. Search only filters immutable cards within that page. Install and
enable the package explicitly to expose its workspace view. The former compiled
gallery and advanced settings showcase are removed.

The static page is rendered at build time using `@flame/ui`. Only native search and the shared theme bootstrap execute in the guest; no React client is shipped. Its geometry is read directly from the icon package during each build.
