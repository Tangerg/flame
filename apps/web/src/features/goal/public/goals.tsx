import { createContext, useContext, useMemo, useSyncExternalStore, type ReactNode } from "react";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { type GoalAccess } from "@/features/goal/application/goalAccess";
import { type GoalCommands } from "@/features/goal/application/goalCommands";

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
  const generation = useSyncExternalStore(commands.subscribe, commands.capture, commands.capture);
  return useMemo(() => {
    const requireGeneration = () => {
      if (!generation) throw new GenerationRetiredError("goal_command_generation");
      return generation;
    };
    return {
      update: (input) => requireGeneration().update(input),
      clear: (sessionId) => requireGeneration().clear(sessionId),
      stop: (sessionId) => requireGeneration().stop(sessionId),
      resume: (sessionId) => requireGeneration().resume(sessionId),
    };
  }, [generation]);
}
