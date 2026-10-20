import { type KeyValueStore } from "@/extensions/storage";
import { type TaskHandle, type TaskStartOptions } from "@/extensions/tasksStore";

export interface AmbientShell {
  readonly storage: KeyValueStore;
  startTask(opts: TaskStartOptions): TaskHandle;
}
