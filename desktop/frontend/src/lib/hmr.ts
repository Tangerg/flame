export function disposeOnHmr(...cleanups: Array<() => void>): void {
  if (!import.meta.hot) return;
  import.meta.hot.dispose(() => {
    for (const c of cleanups) c();
  });
}
