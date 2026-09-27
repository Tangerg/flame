import { SerialQueue } from "dougong";

// A retired view must release callers even when a remote operation ignores cancellation.
// Dougong owns sequencing; the cohort owns identity partitions and publication eligibility.
export class RetirableTaskCohort {
  readonly #retiredError: Error;
  readonly #settlers = new Set<() => void>();
  readonly #queues = new Map<string, SerialQueue>();
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

  runSerial<T>(identity: string, operation: () => PromiseLike<T>): Promise<T> {
    return this.run(() => {
      let queue = this.#queues.get(identity);
      if (!queue) {
        queue = new SerialQueue();
        this.#queues.set(identity, queue);
      }
      const ownedQueue = queue;
      const result = ownedQueue.run(() => this.run(operation));
      const settlement = ownedQueue.settled;
      void settlement.then(() => {
        if (ownedQueue.settled === settlement && this.#queues.get(identity) === ownedQueue) {
          this.#queues.delete(identity);
        }
      });
      return result;
    });
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
    this.#queues.clear();
  }
}
