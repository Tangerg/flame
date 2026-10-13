import type { FlameClient } from "@flame/runtime-contract/client";
import type {
  AgentMemoryItem,
  Page,
  Schedule,
  ReadPluginViewRequest,
  TrajectoryEntry,
} from "@flame/runtime-contract/wire";
import type {
  PluginViewReads,
  MemoryViewTarget,
  SkillViewTarget,
  SkillViewSnapshot,
} from "../application/pluginView";

export function createTrajectoryViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readTrajectory">,
  binding: Readonly<ReadPluginViewRequest>,
  sessionId: string,
  includeDescendants: boolean,
): PluginViewReads<Page<TrajectoryEntry>> {
  const view = Object.freeze({ ...binding });
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readTrajectory(
      {
        ...view,
        sessionId,
        ...(includeDescendants ? { includeDescendants: true } : {}),
        cursor,
        limit: 100,
      },
      signal,
    ),
  );
}

export function createMemoryViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readMemory">,
  binding: Readonly<ReadPluginViewRequest>,
  target: Readonly<MemoryViewTarget>,
): PluginViewReads<Page<AgentMemoryItem>> {
  const view = Object.freeze({ ...binding });
  const captured = {
    ...target,
    ...(target.workspace ? { workspace: { ...target.workspace } } : {}),
  };
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readMemory({ ...view, ...captured, cursor }, signal),
  );
}

export function createScheduleViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readSchedules">,
  binding: Readonly<ReadPluginViewRequest>,
): PluginViewReads<Page<Schedule>> {
  const view = Object.freeze({ ...binding });
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readSchedules({ ...view, cursor }, signal),
  );
}

export function createSkillViewReads(
  plugins: Pick<
    FlameClient["plugins"],
    "readView" | "readSkills" | "readSkill" | "readSkillLibrary" | "readSkillProposals"
  >,
  binding: Readonly<ReadPluginViewRequest>,
  target: Readonly<SkillViewTarget>,
): PluginViewReads<SkillViewSnapshot> {
  const view = Object.freeze({ ...binding });
  const captured =
    target.type === "library" ? { ...target } : { ...target, workspace: { ...target.workspace } };
  return createViewReads(plugins, view, async (cursor, signal): Promise<SkillViewSnapshot> => {
    if (cursor !== undefined)
      throw new Error("Skill catalogs and documents have no cursor continuation");
    switch (captured.type) {
      case "available":
        return {
          type: captured.type,
          data: await plugins.readSkills({ ...view, workspace: captured.workspace }, signal),
        };
      case "detail":
        return {
          type: captured.type,
          data: await plugins.readSkill(
            { ...view, workspace: captured.workspace, name: captured.name },
            signal,
          ),
        };
      case "library":
        return { type: captured.type, data: await plugins.readSkillLibrary(view, signal) };
      case "review":
        return {
          type: captured.type,
          data: await plugins.readSkillProposals(
            { ...view, workspace: captured.workspace },
            signal,
          ),
        };
    }
  });
}

function createViewReads<T>(
  plugins: Pick<FlameClient["plugins"], "readView">,
  view: ReadPluginViewRequest,
  read: (cursor: string | undefined, signal: AbortSignal) => Promise<T>,
): PluginViewReads<T> {
  return {
    binding: view,
    resource: async (signal) => (await plugins.readView(view, signal)).html,
    initial: (signal) => read(undefined, signal),
    read,
  };
}
