import {
  GoalCommandOwner,
  type GoalProjectionRepair,
} from "@/features/goal/application/goalCommands";
import { GoalComposerModeOwner } from "@/features/goal/application/goalComposerMode";
import { type GoalAccess } from "@/features/goal/application/goalAccess";

export function createGoals(repairProjection: GoalProjectionRepair) {
  const commands = new GoalCommandOwner(null, repairProjection);
  const composerMode = new GoalComposerModeOwner();
  const access: GoalAccess = { commands, composerMode };
  return {
    commands,
    composerMode,
    access,
    dispose() {
      composerMode.dispose();
      commands.dispose();
    },
  };
}

export type Goals = ReturnType<typeof createGoals>;
