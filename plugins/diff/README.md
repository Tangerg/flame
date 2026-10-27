# Workspace Diff

The optional workspace component calls canonical `workspace.diff.get` through the
public SDK. Host supplies an immutable workspace reference and file-focus revision;
Runtime owns confinement, Git baselines and read bounds. The package has no backend
process, private application import, HTML carrier or own callable contribution.

Comparison mode, file selection, layout and filters are local presentation state. A fresh
workspace/focus instance defaults to working-tree comparison. Switching comparison
cancels the prior read. Failed reads retain the accepted snapshot and label its
actual comparison. Renames, binary evidence and partial reads retain their canonical
meaning. File rows and the reader heading render the canonical Runtime file status
through the package catalogs; the accepted read keeps its wire values. Host
target/focus replacement retires the render instance and joins SDK I/O.

The general entry starts at a changed-file overview with aggregate text counts.
A file-specific entry passes its normalized path to Runtime instead of searching
a workspace-wide slice. Its independent Files command requests the workspace
overview, retaining the current comparison. A structured read that reaches its
row bound offers a path-scoped raw patch read through the existing public API.
The raw read retains Runtime's byte bound and can fail independently; neither
reader raises a limit or treats a partial result as a clean workspace. Raw patch
headers show only known file identity, without invented status or line counts.
File rows navigate to one reader;
they do not expand every file's code into a single report. The comparison and
refresh controls precede the list filter. In the reader, the current file, position,
previous/next commands and layout stay in the shared persistent control zone.
The accepted comparison, file/workspace scope, real baseline and read time stay
beside that identity in both readers and the overview. Read time records client
acceptance, not a repository revision. A pending or failed request retains these
facts with the accepted content; only a successful response advances them.
Full read feedback belongs to scrolling content. The persistent toolbar keeps a
compact failure cue and Refresh; verbose feedback cannot consume the reading
viewport beside a long filename at large font sizes or zoom.
The persistent filename takes at most two lines. The complete current path and
rename provenance remain selectable in the scrolling content for text, binary
and metadata-only files; complete paths also remain in the overview rows.

Returning from a list-owned reader retains its filter and scroll and focuses the
current row, including after a failed comparison. Only a return that actually
starts a workspace read waits for that response. A failed workspace read keeps
the scoped result and its Files command available for an explicit retry.
Changing or clearing the local filter reveals the beginning of its
results while retaining search focus. Unchanged filters and refreshed reads
preserve the current reading position. A direct file entry leaves Escape to the
host's source-return owner;
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
