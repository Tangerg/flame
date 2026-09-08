import type { Tone } from "@/lib/tone";
import type { Translate } from "@/lib/i18n";
import type { WorkspaceFileChange } from "./workspaceQueries";

interface FileChangeTag {
  letter: "A" | "D" | "M";
  /**
   * What the change MEANS, never what colour it is.
   *
   * This was a Tailwind class name — `"text-warning"` — decided in an application model, which
   * is both a layer violation and a silent one: the moment the theme stopped generating that
   * utility the badge simply lost its colour, and nothing said so. `toneInk` in the design
   * system is the one place a tone becomes ink.
   */
  tone: Tone;
}

type FileChangeLineStats = { kind: "binary" } | { kind: "text"; added: number; removed: number };

export interface FileChangeRowViewModel {
  path: string;
  active: boolean;
  tag: FileChangeTag;
  lineStats: FileChangeLineStats;
}

export interface FileChangesViewModel {
  rows: FileChangeRowViewModel[];
  fileCount: number;
  totalAdded: number;
  totalRemoved: number;
  isEmpty: boolean;
}

const TAG_BY_CHANGE: Record<WorkspaceFileChange["change"], FileChangeTag> = {
  add: { tone: "success", letter: "A" },
  del: { tone: "negative", letter: "D" },
  mod: { tone: "warning", letter: "M" },
};

export function fileChangesViewModel(
  files: readonly WorkspaceFileChange[],
  activePath = "",
): FileChangesViewModel {
  let totalAdded = 0;
  let totalRemoved = 0;

  const rows = files.map((file): FileChangeRowViewModel => {
    const added = file.added ?? 0;
    const removed = file.removed ?? 0;
    totalAdded += added;
    totalRemoved += removed;

    return {
      path: file.path,
      active: file.path === activePath,
      tag: TAG_BY_CHANGE[file.change],
      lineStats: file.binary ? { kind: "binary" } : { kind: "text", added, removed },
    };
  });

  return {
    rows,
    fileCount: files.length,
    totalAdded,
    totalRemoved,
    isEmpty: files.length === 0,
  };
}

export function fileChangesSubtext(
  t: Translate,
  { fileCount }: Pick<FileChangesViewModel, "fileCount">,
): string {
  return t("files.uncommitted", { count: fileCount });
}
