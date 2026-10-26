import { clamp01 } from "@/lib/clamp";
import { type Translate } from "@/lib/i18n/index";
import { type TaskEntry } from "@/extensions/tasksStore";
export interface TaskReadout {
  tasks: TaskEntry[];
  runningCount: number;
  head: TaskEntry;
  label: string;
  title: string;
}

export function taskReadout(t: Translate, tasks: Map<string, TaskEntry>): TaskReadout | null {
  if (tasks.size === 0) return null;

  const list = Array.from(tasks.values()).sort((a, b) => a.startedAt - b.startedAt);
  const running = list.filter((task) => task.status === "running");
  const head = running[0] ?? list.at(-1);
  if (!head) return null;

  return {
    tasks: list,
    runningCount: running.length,
    head,
    label: running.length > 1 ? `${head.label} +${running.length - 1}` : head.label,
    title:
      running.length > 0
        ? t("tasks.title.running", { count: running.length })
        : t("tasks.title.recent"),
  };
}

export function taskProgressPercent(task: TaskEntry): number | null {
  if (task.progress === null || task.status === "failed") return null;
  return Math.round(clamp01(task.progress) * 100);
}
