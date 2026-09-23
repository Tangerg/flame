let open: (() => void) | null = null;

export function setChatSearchOpener(fn: (() => void) | null): void {
  open = fn;
}

export function openChatSearch(): void {
  open?.();
}
