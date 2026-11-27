export class CommandActivity<T extends object> {
  #active?: T;
  readonly #listeners = new Set<() => void>();

  constructor(private readonly lifetime: AbortSignal) {}

  snapshot = () => this.#active;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  async run(command: T, observe: () => Promise<void>): Promise<void> {
    if (this.#active || this.lifetime.aborted) return;
    this.#publish(command);
    try {
      await observe();
    } finally {
      this.#publish(undefined);
    }
  }

  #publish(command: T | undefined) {
    this.#active = command;
    for (const listener of this.#listeners) listener();
  }
}
