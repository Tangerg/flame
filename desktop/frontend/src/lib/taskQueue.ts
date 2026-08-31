/** Owns every settlement admitted by one replaceable generation. Retirement rejects only
 *  genuinely pending work, even when the dependency ignores cancellation. */
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

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
  }
}

/** Serialises per identity; different identities proceed independently. */
export class SerialTaskChain {
  readonly #tails = new Map<string, Promise<void>>();

  chain<T>(identity: string, start: (tail: Promise<void>) => Promise<T>): Promise<T> {
    const result = start(this.#tails.get(identity) ?? Promise.resolve());
    // The tail must NEVER reject: the next call for this identity awaits it, so a rejection
    // would fail work that has not run yet.
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    // Only while still ours: anything queued behind has already replaced it.
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  clear(): void {
    this.#tails.clear();
  }
}
