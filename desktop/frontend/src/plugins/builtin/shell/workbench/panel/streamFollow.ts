import { useSyncExternalStore } from "react";

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
