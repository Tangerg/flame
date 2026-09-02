import {
  ASYNC_OWNERSHIP_RETIRED,
  disposeAsyncIterable,
  settleBeforeAbort,
} from "@/lib/asyncOwnership";
import type { FlameClient } from "@/rpc";

type RuntimeRunStream = Awaited<ReturnType<FlameClient["runs"]["subscribe"]>>;

/**
 * Give an aborting generation immediate ownership release even when the transport opening
 * ignores its signal. A stream which arrives after that release is still an acquired foreign
 * resource, so it is retired rather than dropped.
 */
export async function settleRunStreamOpening(
  opening: Promise<RuntimeRunStream>,
  signal: AbortSignal,
): Promise<RuntimeRunStream | null> {
  const opened = await settleBeforeAbort(opening, signal, retireRunStream);
  return opened === ASYNC_OWNERSHIP_RETIRED ? null : opened;
}

/** The generation is already fenced when this runs, so the retirement is not awaited — abort
 *  remains the authoritative teardown path. What it must not do is reimplement the retirement
 *  itself: `disposeAsyncIterable` bounds the close to the next task, so a cooperative stream
 *  joins immediately and a broken one cannot hold the successor. */
export function retireRunStream(stream: RuntimeRunStream): void {
  void disposeAsyncIterable(stream.events);
}
