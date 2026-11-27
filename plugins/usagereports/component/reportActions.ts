import { type Action } from "./model";

export class ReportActions {
  #active?: Action;
  readonly #listeners = new Set<() => void>();

  constructor(private readonly signal: AbortSignal) {}

  snapshot = () => this.#active;

  canStart(): boolean {
    return !this.#active && !this.signal.aborted;
  }

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  async run(action: Action, execute: () => Promise<void>): Promise<void> {
    if (!this.canStart()) return;
    this.#publish(action);
    try {
      await execute();
    } finally {
      this.#publish(undefined);
    }
  }

  #publish(action: Action | undefined) {
    this.#active = action;
    for (const listener of this.#listeners) listener();
  }
}
