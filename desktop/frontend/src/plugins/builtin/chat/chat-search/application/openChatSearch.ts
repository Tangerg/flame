// A capability of the find bar, not a message on a global event bus: one exists per window,
// so a module-level handle is the whole thing and no caller learns how the overlay mounts.

let open: (() => void) | null = null;

/** Called by the overlay for as long as it is mounted. */
export function setChatSearchOpener(fn: (() => void) | null): void {
  open = fn;
}

export function openChatSearch(): void {
  open?.();
}
