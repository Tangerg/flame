export type ReadState<T> = {
  readonly data: T;
  readonly history: readonly string[];
  readonly pending?: readonly string[];
  readonly error?: string;
};

// This owner advances only the admitted read projection. Commands never enter a guest.
export class ReadView<T> {
  private state: ReadState<T>;
  private readonly port: MessagePort;
  private readonly validate: ((data: T) => void) | undefined;
  private readonly listeners = new Set<() => void>();

  constructor(data: T, port: MessagePort, validate?: (data: T) => void) {
    validate?.(data);
    this.state = { data, history: [] };
    this.port = port;
    this.validate = validate;
    port.addEventListener("message", (event) => {
      const reply = event.data;
      if (this.state.pending === undefined) return;
      if (reply?.type === "data") {
        try {
          this.validate?.(reply.data);
        } catch (error) {
          port.postMessage({
            type: "failure",
            reason:
              error instanceof Error ? error.message : "Unsupported page data.",
          });
          return;
        }
        this.publish({ data: reply.data, history: this.state.pending });
      } else if (reply?.type === "error")
        this.publish({
          ...this.state,
          pending: undefined,
          error: reply.reason,
        });
    });
    port.start();
  }

  getSnapshot = (): ReadState<T> => this.state;
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  read(history: readonly string[] = []): void {
    if (this.state.pending !== undefined) return;
    this.publish({ ...this.state, pending: history, error: undefined });
    const cursor = history.at(-1);
    this.port.postMessage({
      type: "read",
      ...(cursor === undefined ? {} : { cursor }),
    });
  }
  private publish(state: ReadState<T>): void {
    this.state = state;
    for (const listener of this.listeners) listener();
  }
}
