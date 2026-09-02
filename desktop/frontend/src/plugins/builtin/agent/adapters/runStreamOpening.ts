import { disposeAsyncIterable } from "@/lib/asyncOwnership";
import type { FlameClient } from "@/rpc";

type RuntimeRunStream = Awaited<ReturnType<FlameClient["runs"]["subscribe"]>>;

/**
 * Give an aborting generation immediate ownership release even when the
 * transport opening ignores its signal. A stream which arrives after that
 * release is still an acquired foreign resource and must be retired.
 */
export function settleRunStreamOpening(
  opening: Promise<RuntimeRunStream>,
  signal: AbortSignal,
): Promise<RuntimeRunStream | null> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(null);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });

    void opening.then(
      (stream) => {
        if (settled) {
          retireRunStream(stream);
          return;
        }
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(stream);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

/** The generation is already fenced when this runs, so the retirement is not awaited — abort
 *  remains the authoritative teardown path. What it must not do is reimplement the retirement
 *  itself: `disposeAsyncIterable` bounds the close to the next task, so a cooperative stream
 *  joins immediately and a broken one cannot hold the successor. */
export function retireRunStream(stream: RuntimeRunStream): void {
  void disposeAsyncIterable(stream.events);
}
