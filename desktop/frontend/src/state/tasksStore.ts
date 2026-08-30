// Long-running task tracker. `host.tasks.start(...)` registers an entry
// here; the Tasks workspace view reads it to show running work + the latest
// label. Settled tasks linger briefly so the user sees the final state
// before they vanish.

import { nanoid } from "nanoid";
import { create } from "zustand";

/** Handle returned by `host.tasks.start`. All methods are idempotent after a
 *  terminal transition (succeed / fail) — extra calls are no-ops. */
export interface TaskHandle {
  /** Update mid-flight state. `progress` is 0..1 (or null for indeterminate). */
  update: (patch: { progress?: number | null; message?: string | null }) => void;
  /** Mark the task done. The status-bar entry briefly flashes "done" then disappears. */
  succeed: (message?: string) => void;
  /** Mark the task failed. The error surfaces in the status bar; entry disappears after a beat. */
  fail: (error: unknown) => void;
}

export interface TaskStartOptions {
  /** Stable id — defaults to a generated one. Pass an id to allow cross-call updates. */
  id?: string;
  /** One-line label shown in the status bar. */
  label: string;
  /** Optional sub-line shown under the label. */
  message?: string;
  /** 0..1 to start with a determinate bar; omit / null for an indeterminate spinner. */
  progress?: number | null;
}

export type TaskStatus = "running" | "succeeded" | "failed";

export interface TaskEntry {
  id: string;
  label: string;
  /** 0..1 for determinate progress; null for indeterminate (spinner). */
  progress: number | null;
  /** Sub-line shown under the label (optional). */
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

/** Rich process-local lifecycle. Object identity is the task generation; wall
 * time remains presentation data and cannot grant a handle mutation rights. */
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

// How long settled tasks linger before auto-removal — long enough for the
// user to catch the success/error flash, short enough that the status bar
// doesn't pile up with old work.
const TASK_LINGER_MS = 2400;

// Imperative entrypoint used by `host.tasks.start`. Kept here (not in
// host.ts) so the lifecycle — id minting, terminal-state guarding,
// auto-removal timer — can be tested without standing up a Host.
export function startTask(pluginName: string, opts: TaskStartOptions): TaskHandle {
  const store = useTasksStore.getState();
  const id = opts.id ?? `task:${pluginName}:${nanoid(8)}`;
  const task = new TaskLifecycle(id, opts);
  store.add(task);

  // Mark settled + schedule removal. Guards against double-settle so a
  // late `succeed()` after `fail()` (or vice versa) is a silent no-op.
  const settle = (transition: (task: TaskLifecycle) => boolean): void => {
    if (!useTasksStore.getState().mutate(task, () => transition(task))) return;
    // The linger timer removes only THE settle it was armed for — a
    // restarted task reusing this id must not be deleted mid-flight by the
    // previous settle's stale timer.
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
