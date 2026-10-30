import { type GoalCommandAccess } from "@/features/goal/application/goalCommands";
import { type GoalComposerModeOwner } from "@/features/goal/application/goalComposerMode";

export interface GoalAccess {
  readonly commands: GoalCommandAccess;
  readonly composerMode: Pick<
    GoalComposerModeOwner,
    "snapshot" | "subscribe" | "deactivate" | "confirmReplacement" | "cancelReplacement"
  >;
}
