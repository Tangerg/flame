/** Arbitrary-precision, process-local monotonic sequence.
 *
 * Use this only when an identity must be encoded as a stable scalar. Pure
 * cancellation/commit authority should use object identity instead.
 */
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
