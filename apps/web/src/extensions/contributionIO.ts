import { asyncDisposeSymbol } from "dougong";
import { type ContributionLifetime } from "@/extensions/definePlugin";
// Admission precedes the operation. The caller observes the original Promise;
// the presentation contribution joins that exact work through its cleanup.
export function trackContributionIO<P extends Promise<unknown>>(
  lifetime: ContributionLifetime,
  start: (signal: AbortSignal) => P,
): P {
  lifetime.signal.throwIfAborted();
  let complete!: () => void;
  const joined = new Promise<void>((resolve) => {
    complete = resolve;
  });
  const cleanup = lifetime.cleanup(() => joined);
  const settled = () => {
    complete();
    void cleanup[asyncDisposeSymbol]();
  };
  try {
    const original = start(lifetime.signal);
    void original.then(settled, settled);
    return original;
  } catch (error) {
    settled();
    throw error;
  }
}
