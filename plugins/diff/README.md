# Workspace Diff

The optional workspace component calls canonical `workspace.diff.get` through the
public SDK. Host supplies an immutable workspace reference and file-focus revision;
Runtime owns confinement, Git baselines and read bounds. The package has no backend
process, private application import, HTML carrier or own callable contribution.

Comparison mode, layout, filters and folding are local presentation state. A fresh
workspace/focus instance defaults to working-tree comparison. Switching comparison
cancels the prior read. Failed reads retain the accepted snapshot and label its
actual comparison. Renames, binary evidence and partial reads retain their canonical
meaning. Host target/focus replacement retires the render instance and joins SDK I/O.

The compact toolbar keeps comparison, layout, refresh and global folding available
while the report scrolls. File summaries show one complete path with compact status
and count metadata; directory separators provide natural wrapping points.
Each file heading stays below the controls while its body scrolls when the reading
viewport has room for the complete heading and two lines of body. Taller headings
remain in normal flow. Focus clearance uses the same measured heading height.

The generic material key `diff` participates in the existing workspace navigation
consumer. Absent/competing candidates open the catalogue. Disablement/uninstall
withdraws registrations and package resources without changing files, Git history,
recorded Tool results or independent Runs. An evaluated release replacement requires
the normal window reload.

Build icons and UI, install the portable toolchain, then run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

Edit `src/component.tsx` and rebuild the tracked ESM. React/JSX/UI are host-provided
shared modules. Installation admits exact package bytes without executing code.
