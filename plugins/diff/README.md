# Workspace Diff

The optional workspace component calls canonical `workspace.diff.get` through the
public SDK. Host supplies an immutable workspace reference and file-focus revision;
Runtime owns confinement, Git baselines and read bounds. The package has no backend
process, private application import, HTML carrier or own callable contribution.

Comparison mode, file selection, layout and filters are local presentation state. A fresh
workspace/focus instance defaults to working-tree comparison. Switching comparison
cancels the prior read. Failed reads retain the accepted snapshot and label its
actual comparison. Renames, binary evidence and partial reads retain their canonical
meaning. Host target/focus replacement retires the render instance and joins SDK I/O.

The general entry starts at a changed-file overview with aggregate text counts.
A file-specific entry opens that file directly. File rows navigate to one reader;
they do not expand every file's code into a single report. The comparison and
refresh controls precede the list filter. In the reader, the current file, position,
previous/next commands and layout stay in the shared persistent control zone.
Complete directories and rename provenance remain in the scrolling content.

Returning from a list-owned reader retains its query and scroll and focuses the
current row. A direct file entry leaves Escape to the host's source-return owner;
its independent Files command opens the overview. Layout popups consume their own
dismissal first. Refresh retains the chosen reader when it remains in the accepted
comparison; an absent file returns to the overview with an explicit notice. Failed
reads keep the accepted comparison visibly identified beside the scope control.
The package remains a read-only current-workspace view, not a stored Tool diff.

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

Edit `component/component.tsx` and rebuild the tracked ESM. React/JSX/UI are host-provided
shared modules. Installation admits exact package bytes without executing code.
