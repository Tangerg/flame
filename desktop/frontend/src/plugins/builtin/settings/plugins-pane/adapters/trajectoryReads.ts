import type { FlameClient } from "@flame/runtime-contract/client";
import type { ReadPluginViewRequest } from "@flame/runtime-contract/wire";
import type { TrajectoryViewReads } from "../application/trajectoryView";

export function createTrajectoryViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readTrajectory">,
  binding: Readonly<ReadPluginViewRequest>,
  sessionId: string,
): TrajectoryViewReads {
  const view = { ...binding };
  const read = (cursor: string | undefined, signal: AbortSignal) =>
    plugins.readTrajectory({ ...view, sessionId, cursor }, signal);
  return {
    read,
    async load(signal) {
      const loading = new AbortController();
      const owned = AbortSignal.any([signal, loading.signal]);
      const pending = [plugins.readView(view, owned), read(undefined, owned)] as const;
      try {
        const [resource, initial] = await Promise.all(pending);
        return { html: resource.html, initial };
      } catch (error) {
        loading.abort(error);
        await Promise.allSettled(pending);
        throw error;
      }
    },
  };
}
