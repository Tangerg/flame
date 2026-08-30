// Vite's HMR replaces a module's exports in place but does NOT undo side effects it already
// ran, so a module-level `subscribe(...)` keeps firing through stale closures and each
// reload stacks another listener. Required by CLAUDE.md §5 for any module-level subscribe.
//
// In production `import.meta.hot` is undefined and the body is dead-code-eliminated.

export function disposeOnHmr(...cleanups: Array<() => void>): void {
  if (!import.meta.hot) return;
  import.meta.hot.dispose(() => {
    for (const c of cleanups) c();
  });
}
