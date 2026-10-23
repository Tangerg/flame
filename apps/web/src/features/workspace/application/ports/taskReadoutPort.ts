import { createSingletonPort } from "@/lib/ports/singletonPort";
import { type TaskEntry } from "@/extensions/tasksStore";
// The task shape is owned by the producing store; the port only inverts the
// runtime dependency, so it re-exports the type rather than restating it.
export type { TaskEntry, TaskStatus } from "@/extensions/tasksStore";

interface TaskReadoutPort {
  useTasks(): Map<string, TaskEntry>;
}

const port = createSingletonPort<TaskReadoutPort>("task readout port is not configured");

export const configureTaskReadoutPort = port.configure;

export const taskReadoutPort = port.get;
