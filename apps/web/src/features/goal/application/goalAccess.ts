import { type GoalCommandAccess } from "@/features/goal/application/goalCommands";
import { type GoalComposerSubmissionOwner } from "@/features/goal/application/goalComposerSubmission";

export interface GoalAccess {
  readonly commands: GoalCommandAccess;
  readonly submissions: Pick<
    GoalComposerSubmissionOwner,
    "snapshot" | "subscribe" | "confirmReplacement" | "cancelReplacement"
  >;
}
