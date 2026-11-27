import { failureText } from "./errorText";

export type PageDirection = "next" | "previous";

interface PageValue {
  readonly next?: string;
}

type PageRead<T> = Readonly<
  { cursors: readonly string[] } & (
    | { loading: true; value?: T; error?: never }
    | { loading: false; value: T; error?: never }
    | { loading: false; value?: T; error: string }
  )
>;

type ReadPage<T> = (before: string | undefined, signal: AbortSignal) => Promise<T>;

export class CursorPager<T extends PageValue> {
  // Background invalidation follows the requested cursor while a read retains the old page.
  #requested: readonly string[] = [];
  #page?: PageRead<T>;
  #request?: AbortController;
  readonly #listeners = new Set<() => void>();

  constructor(
    private readonly signal: AbortSignal,
    private readonly read?: ReadPage<T>,
  ) {
    if (read) this.#page = { loading: true, cursors: [] };
  }

  snapshot = () => this.#page;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  stop = () => this.#request?.abort();

  move(direction: PageDirection) {
    const page = this.#page;
    if (!page || page.loading || this.signal.aborted) return;
    if (direction === "next") {
      const next = page.value?.next;
      if (!next) return;
      this.#requested = [...page.cursors, next];
    } else {
      if (!page.cursors.length) return;
      this.#requested = page.cursors.slice(0, -1);
    }
    return this.refresh();
  }

  refresh = async () => {
    if (!this.read || this.signal.aborted) return;
    this.stop();
    const cursors = this.#requested;
    const request = new AbortController();
    this.#request = request;
    const signal = AbortSignal.any([this.signal, request.signal]);
    const retained = {
      value: this.#page?.value,
      cursors: this.#page?.cursors ?? [],
    };
    this.#publish({ ...retained, loading: true });
    try {
      const value = await this.read(cursors.at(-1), signal);
      if (!signal.aborted) this.#publish({ value, cursors, loading: false });
    } catch (error) {
      if (!signal.aborted)
        this.#publish({ ...retained, loading: false, error: failureText(error) });
    } finally {
      if (this.#request === request) this.#request = undefined;
    }
  };

  #publish(page: PageRead<T>) {
    this.#page = page;
    for (const listener of this.#listeners) listener();
  }
}
