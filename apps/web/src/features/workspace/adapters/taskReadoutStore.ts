import { useTasksStore } from "@/extensions/tasksStore";
import { configureTaskReadoutPort } from "@/features/workspace/application/ports/taskReadoutPort";
export function installTaskReadoutPort(): () => void {
  return configureTaskReadoutPort({
    useTasks: () => useTasksStore((state) => state.tasks),
  });
}
