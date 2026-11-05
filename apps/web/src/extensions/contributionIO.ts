import { asyncDisposeSymbol } from "dougong";
import { type ContributionScope } from "@/extensions/definePlugin";
// Admission precedes the operation. The caller observes the original Promise;
// the presentation contribution joins that exact work through its cleanup.
export function trackContributionIO<P extends Promise<unknown>>(
  lifetime: ContributionScope,
  start: (signal: AbortSignal) => P,
): P {
  lifetime.assertOpen();
  let complete!: () => void;
  const joined = new Promise<void>((resolve) => {
    complete = resolve;
  });
  const cleanup = lifetime.lifetime.defer(() => joined);
  const settled = () => {
    complete();
    void cleanup[asyncDisposeSymbol]();
  };
  try {
    const original = start(lifetime.lifetime.signal);
    void original.then(settled, settled);
    return original;
  } catch (error) {
    settled();
    throw error;
  }
}
