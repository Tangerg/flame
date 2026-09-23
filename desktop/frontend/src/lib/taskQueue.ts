export class RetirableTaskCohort {
  readonly #retiredError: Error;
  readonly #settlers = new Set<() => void>();
  #retired = false;

  constructor(retiredError: Error) {
    this.#retiredError = retiredError;
  }

  get retired(): boolean {
    return this.#retired;
  }

  assertCurrent(): void {
    if (this.#retired) throw this.#retiredError;
  }

  settle<T>(operation: PromiseLike<T>): Promise<T> {
    this.assertCurrent();
    return new Promise<T>((resolve, reject) => {
      let pending = true;
      const finish = () => {
        if (!pending) return false;
        pending = false;
        this.#settlers.delete(retire);
        return true;
      };
      const retire = () => {
        if (finish()) reject(this.#retiredError);
      };
      this.#settlers.add(retire);
      operation.then(
        (value) => {
          if (finish()) resolve(value);
        },
        (error: unknown) => {
          if (finish()) reject(error);
        },
      );
      if (this.#retired) retire();
    });
  }

  async run<T>(operation: () => PromiseLike<T>): Promise<T> {
    this.assertCurrent();
    const value = await this.settle(operation());
    this.assertCurrent();
    return value;
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
  }
}

export class SerialTaskChain {
  readonly #tails = new Map<string, Promise<void>>();

  chain<T>(identity: string, start: (tail: Promise<void>) => Promise<T>): Promise<T> {
    const result = start(this.#tails.get(identity) ?? Promise.resolve());
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  clear(): void {
    this.#tails.clear();
  }
}
