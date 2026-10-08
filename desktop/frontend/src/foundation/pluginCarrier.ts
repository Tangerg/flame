export interface PluginPage {
  send(message: unknown): Promise<void>;
  close(): Promise<void>;
}

// The carrier supplies a source-bound transport; the feature owns read authority.
export interface PluginCarrier {
  open(
    container: HTMLElement,
    signal: AbortSignal,
    receive: (message: unknown) => void,
  ): Promise<PluginPage>;
}
