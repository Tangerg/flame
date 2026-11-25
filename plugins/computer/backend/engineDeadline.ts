import { performance } from "node:perf_hooks";

export const CONNECT_MS = 120_000;

export const CLEANUP_MS = 120_000;

export const CALL_MS = 120_000;

export const RESULT_MARGIN_MS = 30_000;

export const MAX_TIMER_MS = 2 ** 31 - 1;

export class ExecutionDeadline {
  readonly controller = new AbortController();
  readonly #parent: AbortSignal;
  #remaining: number;
  #started = performance.now();
  #timer?: ReturnType<typeof setTimeout>;
  #paused = 0;
  #closed = false;

  constructor(milliseconds: number, parent: AbortSignal) {
    this.#remaining = milliseconds;
    this.#parent = parent;
    if (parent.aborted) this.#abort();
    else {
      parent.addEventListener("abort", this.#abort, { once: true });
      this.#resume();
    }
  }

  readonly #abort = (): void => {
    this.controller.abort(this.#parent.reason);
    this.close();
  };

  #resume(): void {
    if (this.#closed || this.#paused || this.controller.signal.aborted) return;
    this.#started = performance.now();
    this.#timer = setTimeout(
      () => {
        this.controller.abort(
          new Error("computer: engine execution deadline elapsed"),
        );
        this.close();
      },
      Math.max(0, this.#remaining),
    );
  }

  pause(): () => void {
    if (this.#closed) return () => {};
    if (this.#paused++ === 0) {
      clearTimeout(this.#timer);
      this.#remaining -= performance.now() - this.#started;
    }
    let resumed = false;
    return () => {
      if (resumed || this.#closed) return;
      resumed = true;
      this.#paused--;
      this.#resume();
    };
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearTimeout(this.#timer);
    this.#parent.removeEventListener("abort", this.#abort);
  }
}
