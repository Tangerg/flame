import { nanoid } from "nanoid";
import { create } from "zustand";

/** Idempotent after a terminal transition: extra calls are no-ops. */
export interface TaskHandle {
  /** `progress` is 0..1, or null for indeterminate. */
  update: (patch: { progress?: number | null; message?: string | null }) => void;
  succeed: (message?: string) => void;
  fail: (error: unknown) => void;
}

export interface TaskStartOptions {
  /** Pass an id to allow cross-call updates; defaults to a generated one. */
  id?: string;
  label: string;
  message?: string;
  progress?: number | null;
}

export type TaskStatus = "running" | "succeeded" | "failed";

export interface TaskEntry {
  id: string;
  label: string;
  /** 0..1, or null for indeterminate. */
  progress: number | null;
  message: string | null;
  status: TaskStatus;
  /** Populated when `status === "failed"`. */
  error?: string;
  startedAt: number;
  /** Set on terminal transitions; the store removes the entry shortly after. */
  settledAt?: number;
}

interface TasksState {
  tasks: Map<string, TaskEntry>;
}

interface TasksActions {
  add: (task: TaskLifecycle) => void;
  mutate: (task: TaskLifecycle, mutation: () => boolean) => boolean;
  remove: (task: TaskLifecycle) => void;
}

export const useTasksStore = create<TasksState & TasksActions>((set) => ({
  tasks: new Map(),
  add: (task) =>
    set((s) => {
      const next = new Map(s.tasks);
      next.set(task.id, task);
      return { tasks: next };
    }),
  mutate: (task, mutation) => {
    let accepted = false;
    set((s) => {
      if (s.tasks.get(task.id) !== task || !mutation()) return s;
      accepted = true;
      return { tasks: new Map(s.tasks) };
    });
    return accepted;
  },
  remove: (task) =>
    set((s) => {
      if (s.tasks.get(task.id) !== task) return s;
      const next = new Map(s.tasks);
      next.delete(task.id);
      return { tasks: next };
    }),
}));

/** Object identity is the task generation; wall time is presentation data and cannot grant
 *  a handle mutation rights. */
class TaskLifecycle implements TaskEntry {
  readonly id: string;
  readonly label: string;
  readonly startedAt: number;
  progress: number | null;
  message: string | null;
  status: TaskStatus = "running";
  error?: string;
  settledAt?: number;

  constructor(id: string, opts: TaskStartOptions) {
    this.id = id;
    this.label = opts.label;
    this.message = opts.message ?? null;
    this.progress = opts.progress ?? null;
    this.startedAt = Date.now();
  }

  update(patch: { progress?: number | null; message?: string | null }): boolean {
    if (this.status !== "running") return false;
    if (patch.progress !== undefined) this.progress = patch.progress;
    if (patch.message !== undefined) this.message = patch.message;
    return true;
  }

  succeed(message?: string): boolean {
    if (!this.settle("succeeded")) return false;
    this.progress = 1;
    if (message !== undefined) this.message = message;
    return true;
  }

  fail(error: unknown): boolean {
    if (!this.settle("failed")) return false;
    this.error = error instanceof Error ? error.message : String(error);
    return true;
  }

  private settle(status: Exclude<TaskStatus, "running">): boolean {
    if (this.status !== "running") return false;
    this.status = status;
    this.settledAt = Date.now();
    return true;
  }
}

const TASK_LINGER_MS = 2400;

export function startTask(pluginName: string, opts: TaskStartOptions): TaskHandle {
  const store = useTasksStore.getState();
  const id = opts.id ?? `task:${pluginName}:${nanoid(8)}`;
  const task = new TaskLifecycle(id, opts);
  store.add(task);

  const settle = (transition: (task: TaskLifecycle) => boolean): void => {
    if (!useTasksStore.getState().mutate(task, () => transition(task))) return;
    // The timer removes only THE settle it was armed for: a restarted task reusing this id
    // must not be deleted mid-flight by the previous settle's stale timer.
    window.setTimeout(() => {
      useTasksStore.getState().remove(task);
    }, TASK_LINGER_MS);
  };

  return {
    update(patch) {
      useTasksStore.getState().mutate(task, () => task.update(patch));
    },
    succeed(message) {
      settle((current) => current.succeed(message));
    },
    fail(err) {
      settle((current) => current.fail(err));
    },
  };
}
