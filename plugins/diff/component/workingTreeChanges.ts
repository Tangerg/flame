import type { WorkspaceChangesActionInput } from "@flame/component-sdk";
type RecordedChange = WorkspaceChangesActionInput["changes"][number];
interface WorkingTreeChanges {
  files: number;
  added: number | undefined;
  removed: number | undefined;
}
export function summarizeWorkingTreeChanges(
  data: readonly RecordedChange[] | undefined,
): WorkingTreeChanges | null {
  if (!data || data.length === 0) return null;
  return data.reduce<WorkingTreeChanges>(
    (sum, file) => ({
      files: sum.files,
      added:
        sum.added === undefined || file.added === undefined
          ? undefined
          : sum.added + file.added,
      removed:
        sum.removed === undefined || file.removed === undefined
          ? undefined
          : sum.removed + file.removed,
    }),
    { files: data.length, added: 0, removed: 0 },
  );
}
