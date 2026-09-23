import {
  ASYNC_OWNERSHIP_RETIRED,
  disposeAsyncIterable,
  settleBeforeAbort,
} from "@/lib/asyncOwnership";
import type { FlameClient } from "@/rpc";

type RuntimeRunStream = Awaited<ReturnType<FlameClient["runs"]["subscribe"]>>;

export async function settleRunStreamOpening(
  opening: Promise<RuntimeRunStream>,
  signal: AbortSignal,
): Promise<RuntimeRunStream | null> {
  const opened = await settleBeforeAbort(opening, signal, retireRunStream);
  return opened === ASYNC_OWNERSHIP_RETIRED ? null : opened;
}

export function retireRunStream(stream: RuntimeRunStream): void {
  void disposeAsyncIterable(stream.events);
}
