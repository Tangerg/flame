import { useSyncExternalStore } from "react";

type Listener = () => void;

let wrapCode = false;
const listeners = new Set<Listener>();

function subscribe(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function snapshot(): boolean {
  return wrapCode;
}

export function useCodeWrapPreference(): boolean {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

export function setCodeWrapPreference(next: boolean): void {
  if (wrapCode === next) return;
  wrapCode = next;
  for (const listener of listeners) listener();
}

export function toggleCodeWrapPreference(): void {
  setCodeWrapPreference(!wrapCode);
}
