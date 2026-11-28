import { GoalCommandOwner } from "@/features/goal/application/goalCommands";
import { GoalComposerSubmissionOwner } from "@/features/goal/application/goalComposerSubmission";
import { type GoalAccess } from "@/features/goal/application/goalAccess";

export function createGoals() {
  const commands = new GoalCommandOwner();
  const submissions = new GoalComposerSubmissionOwner();
  const access: GoalAccess = { commands, submissions };
  return {
    commands,
    submissions,
    access,
    dispose() {
      submissions.dispose();
      commands.dispose();
    },
  };
}

export type Goals = ReturnType<typeof createGoals>;
