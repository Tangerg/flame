import { useSyncExternalStore } from "react";

// A PASSIVE snapshot, not state on the surrounding component: `use-stick-to-bottom` rebuilds
// its context object on every scroll event, so lifting it through the parent re-renders the
// entire chat surface for ordinary scrolling.
//
// Only `atBottom` is reactive. `scrollToBottom` is read from a click handler, so publishing
// a new one notifies nobody — a scroll that crosses no threshold re-renders nothing.

let atBottom = true;
let scrollToBottom = (): void => {};
const listeners = new Set<() => void>();

export function publishStreamFollow(next: { atBottom: boolean; scrollToBottom: () => void }): void {
  scrollToBottom = next.scrollToBottom;
  if (next.atBottom === atBottom) return;
  atBottom = next.atBottom;
  for (const listener of listeners) listener();
}

export function scrollStreamToBottom(): void {
  scrollToBottom();
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

export function useStreamAtBottom(): boolean {
  return useSyncExternalStore(subscribe, () => atBottom);
}
