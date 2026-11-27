import { createContext, useContext, useMemo, useSyncExternalStore, type ReactNode } from "react";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { type GoalAccess } from "@/features/goal/application/goalAccess";
import { GOAL_COMMAND_SCOPE, type GoalCommands } from "@/features/goal/application/goalCommands";

const GoalContext = createContext<GoalAccess | null>(null);

export function GoalProvider({ value, children }: { value: GoalAccess; children: ReactNode }) {
  return <GoalContext value={value}>{children}</GoalContext>;
}

export function useGoalAccess(): GoalAccess {
  const value = useContext(GoalContext);
  if (!value) throw new Error("GoalProvider is missing");
  return value;
}

export function useGoalCommands(): Pick<GoalCommands, "update" | "clear" | "stop" | "resume"> {
  const { commands } = useGoalAccess();
  const scope = useSyncExternalStore(commands.subscribe, commands.capture, commands.capture);
  return useMemo(() => {
    const requireScope = () => {
      if (!scope) throw new GenerationRetiredError(GOAL_COMMAND_SCOPE);
      return scope;
    };
    return {
      update: (input) => requireScope().update(input),
      clear: (target) => requireScope().clear(target),
      stop: (target) => requireScope().stop(target),
      resume: (target) => requireScope().resume(target),
    };
  }, [scope]);
}
