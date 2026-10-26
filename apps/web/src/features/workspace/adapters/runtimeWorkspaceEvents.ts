import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type FlameClient, type RuntimeTopic } from "@flame/runtime-client";
import { RUNTIME_SUBSCRIBE_METHOD } from "@flame/runtime-client";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
  runtimeSupportsTopic,
} from "@/features/connection/public/capabilities";
import { type WorkspaceWatchTarget } from "@/features/workspace/application/workspaceEventLoop";
import {
  type WorkspaceEventLike,
  type WorkspaceWatchScope,
} from "@/features/workspace/domain/eventInvalidation";
const SUBSCRIBED_TOPICS: readonly RuntimeTopic[] = [
  "files.changed",
  "skills.changed",
  "plugins.changed",
  "mcp.changed",
  "plugins.dataChanged",
  "sessions.changed",
  "runs.changed",
  "interrupts.changed",
  "goals.changed",
  "plan.changed",
  "hooks.changed",
  "models.changed",
  "approvals.changed",
];

export function canSubscribeWorkspaceEvents(runtimeAccess: RuntimeAccess): boolean {
  return runtimeSupportsStreamingMethod(runtimeAccess, RUNTIME_SUBSCRIBE_METHOD);
}

export async function subscribeRuntimeWorkspaceEvents(
  runtimeAccess: RuntimeAccess,
  runtimeClient: () => FlameClient,
  target: WorkspaceWatchTarget,
  signal: AbortSignal,
): Promise<AsyncIterable<WorkspaceEventLike>> {
  const client = runtimeClient();
  const requested = new Map<
    string | undefined,
    { watchId: string; cwd?: string; paths: Set<string> }
  >();
  if (runtimeCapability(runtimeAccess, "fileWatch")) {
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
  const topics = SUBSCRIBED_TOPICS.filter(runtimeSupportsTopic.bind(null, runtimeAccess));
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
