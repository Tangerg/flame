export interface MountedRenderer {
  unmount(): void;
}

export interface ClientRendererDependencies {
  initializeClientHost(): Promise<void>;
  prepareWindowChrome(): Promise<void>;
  watchWindowChrome(): () => void;
  mount(): MountedRenderer;
  mountStartupFailure(error: unknown): MountedRenderer;
  closeConnection(): Promise<void>;
  reportFailure(scope: "React root teardown" | "window chrome teardown", error: unknown): void;
}

export class ClientRenderer {
  readonly #dependencies: ClientRendererDependencies;
  #active = true;
  #startup: Promise<void> | undefined;
  #mounted: MountedRenderer | undefined;
  #stopWatchingWindowChrome: (() => void) | undefined;
  #closing: Promise<void> | undefined;

  constructor(dependencies: ClientRendererDependencies) {
    this.#dependencies = dependencies;
  }

  start(): Promise<void> {
    if (this.#startup) return this.#startup;
    if (!this.#active) return Promise.resolve();
    const startup = this.#startOwned().catch(async (error: unknown) => {
      this.#retireMounted();
      let failure = error;
      try {
        await this.#closeConnection();
      } catch (closeError) {
        failure = new AggregateError([error, closeError], "Client startup and cleanup failed");
      }
      if (this.#active) {
        const mounted = this.#dependencies.mountStartupFailure(failure);
        if (this.#active) this.#mounted = mounted;
        else mounted.unmount();
      }
      throw failure;
    });
    this.#startup = startup;
    return startup;
  }

  async #startOwned(): Promise<void> {
    await this.#dependencies.initializeClientHost();
    if (!this.#active) return;

    await this.#dependencies.prepareWindowChrome();
    if (!this.#active) return;

    const stopWatching = this.#dependencies.watchWindowChrome();
    if (!this.#active) {
      stopWatching();
      return;
    }
    this.#stopWatchingWindowChrome = stopWatching;

    const mounted = this.#dependencies.mount();
    if (!this.#active) {
      mounted.unmount();
      return;
    }
    this.#mounted = mounted;
  }

  dispose(): Promise<void> {
    this.#active = false;
    this.#retireMounted();
    return this.#closeConnection();
  }

  #retireMounted(): void {
    const mounted = this.#mounted;
    this.#mounted = undefined;
    try {
      mounted?.unmount();
    } catch (error) {
      this.#dependencies.reportFailure("React root teardown", error);
    }

    const stopWatching = this.#stopWatchingWindowChrome;
    this.#stopWatchingWindowChrome = undefined;
    try {
      stopWatching?.();
    } catch (error) {
      this.#dependencies.reportFailure("window chrome teardown", error);
    }
  }

  #closeConnection(): Promise<void> {
    if (this.#closing) return this.#closing;
    try {
      this.#closing = this.#dependencies.closeConnection();
    } catch (error) {
      this.#closing = Promise.reject(error);
    }
    return this.#closing;
  }
}
