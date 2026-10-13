import { asyncDisposeSymbol } from "dougong";
import type { ContributionLifetime } from "./definePlugin";

// The caller observes the outcome; its contribution joins the same I/O promise.
export function trackContributionIO<P extends Promise<unknown>>(
  lifetime: ContributionLifetime,
  operation: P,
): P {
  const join = () =>
    operation.then(
      () => undefined,
      () => undefined,
    );
  const cleanup = lifetime.cleanup(join);
  void join().then(() => cleanup[asyncDisposeSymbol]());
  return operation;
}
