import type { RuntimeEvent, RuntimeTopic } from "@/rpc";
import { getContainer } from "@/main/container";
import { RUNTIME_SUBSCRIBE_METHOD } from "@/rpc/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
  runtimeSupportsTopic,
} from "@/plugins/builtin/runtime/public/capabilities";
import type { WorkspaceWatchTarget } from "../application/workspaceEventLoop";

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
): Promise<AsyncIterable<RuntimeEvent>> {
  const client = getContainer().client();
  const workspace =
    runtimeCapability("fileWatch") && target.type === "workspace"
      ? await client.workspaces.resolve(target.cwd ? { path: target.cwd } : undefined, signal)
      : undefined;
  const watches =
    workspace?.availability === "available"
      ? [{ watchId: "active-session", workspace: { path: workspace.ref.path } }]
      : undefined;
  const topics = SUBSCRIBED_TOPICS.filter(runtimeSupportsTopic);
  const { events } = await client.runtimeEvents.subscribe(
    { topics, ...(watches ? { watches } : {}) },
    signal,
  );
  return events;
}
