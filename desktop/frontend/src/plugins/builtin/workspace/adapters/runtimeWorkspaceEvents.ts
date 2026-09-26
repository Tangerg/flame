import type { RuntimeTopic } from "@flame/runtime-contract/client";
import { getContainer } from "@/main/container";
import { RUNTIME_SUBSCRIBE_METHOD } from "@flame/runtime-contract/client/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
  runtimeSupportsTopic,
} from "@/plugins/builtin/runtime/public/capabilities";
import type { WorkspaceWatchTarget } from "../application/workspaceEventLoop";
import type { WorkspaceEventLike, WorkspaceWatchScope } from "../domain/eventInvalidation";

const SUBSCRIBED_TOPICS: readonly RuntimeTopic[] = [
  "files.changed",
  "skills.changed",
  "mcp.changed",
  "schedules.changed",
  "sessions.changed",
  "runs.changed",
  "interrupts.changed",
  "goals.changed",
  "plan.changed",
  "hooks.changed",
  "models.changed",
  "approvals.changed",
  "agentMemory.changed",
];

export function canSubscribeWorkspaceEvents(): boolean {
  return runtimeSupportsStreamingMethod(RUNTIME_SUBSCRIBE_METHOD);
}

export async function subscribeRuntimeWorkspaceEvents(
  target: WorkspaceWatchTarget,
  signal: AbortSignal,
): Promise<AsyncIterable<WorkspaceEventLike>> {
  const client = getContainer().client();
  const requested = new Map<
    string | undefined,
    { watchId: string; cwd?: string; paths: Set<string> }
  >();
  if (runtimeCapability("fileWatch")) {
    if (target.type === "workspace") {
      requested.set(target.cwd, { watchId: "active-session", cwd: target.cwd, paths: new Set() });
    }
    for (const read of target.reads ?? []) {
      const scope = requested.get(read.cwd) ?? {
        watchId: `open-reads-${requested.size}`,
        cwd: read.cwd,
        paths: new Set<string>(),
      };
      for (const path of read.paths) scope.paths.add(path);
      requested.set(read.cwd, scope);
    }
  }
  const watchScopes: WorkspaceWatchScope[] = [];
  const resolved = await Promise.all(
    [...requested.values()].map(async (scope) => {
      const workspace = await client.workspaces.resolve(
        scope.cwd ? { path: scope.cwd } : undefined,
        signal,
      );
      if (workspace.availability !== "available") return undefined;
      const paths = [...scope.paths].map((path) => {
        for (const root of [scope.cwd, workspace.ref.path]) {
          if (root && path === root) return ".";
          if (root && path.startsWith(`${root}/`)) return path.slice(root.length + 1);
        }
        return path;
      });
      watchScopes.push({ watchId: scope.watchId, workspace: workspace.ref, cwd: scope.cwd });
      return {
        watchId: scope.watchId,
        workspace: workspace.ref,
        ...(paths.length ? { paths } : {}),
      };
    }),
  );
  const watches = resolved.filter((watch) => watch !== undefined);
  const topics = SUBSCRIBED_TOPICS.filter(runtimeSupportsTopic);
  const { events } = await client.runtimeEvents.subscribe(
    { topics, ...(watches.length ? { watches } : {}) },
    signal,
  );
  if (!watchScopes.length) return events;
  return {
    [Symbol.asyncIterator]() {
      const source = events[Symbol.asyncIterator]();
      return {
        async next() {
          const next = await source.next();
          return next.done ? next : { done: false, value: { ...next.value, watchScopes } };
        },
        async return() {
          await source.return?.();
          return { done: true, value: undefined };
        },
      };
    },
  };
}
