export class ExactSequence {
  #lastIssued: bigint;

  constructor(lastIssued = 0n) {
    if (lastIssued < 0n) throw new RangeError("Exact sequence cannot start below zero");
    this.#lastIssued = lastIssued;
  }

  issue(): bigint {
    this.#lastIssued += 1n;
    return this.#lastIssued;
  }
}
