// The Find command and the mounted overlay of one plugin installation share
// this opener; only the overlay that attached last can be opened.
export class ChatSearchOpener {
  #open: (() => void) | null = null;

  attach(open: () => void): () => void {
    this.#open = open;
    return () => {
      if (this.#open === open) this.#open = null;
    };
  }

  open(): void {
    this.#open?.();
  }
}
