/**
 * Owns every asynchronous settlement admitted by one replaceable task
 * generation. Completed work unregisters immediately; retirement rejects only
 * genuinely pending work, even when the underlying dependency ignores
 * cancellation.
 */
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

/**
 * Serialises work per identity: a call waits for whatever is already in flight for the SAME
 * identity, while different identities proceed independently. Four mutation owners each
 * carried a private copy of this, and none of them wrote down why it is shaped this way.
 */
export class SerialTaskChain {
  readonly #tails = new Map<string, Promise<void>>();

  chain<T>(identity: string, start: (tail: Promise<void>) => Promise<T>): Promise<T> {
    const result = start(this.#tails.get(identity) ?? Promise.resolve());
    // The tail must NEVER reject. It is what the next call for this identity waits on, so a
    // rejected tail would fail work that has not even run yet — one failed save turning the
    // next, unrelated one into a failure too.
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    // Forget the tail only while it is still ours. Anything queued behind this call has
    // already replaced it, and deleting that would let a third call start before the second
    // finished — which is the serialisation this exists to provide.
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  clear(): void {
    this.#tails.clear();
  }
}
