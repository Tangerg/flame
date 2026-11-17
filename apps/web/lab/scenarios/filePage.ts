import type {
  WorkspaceFileEntry,
  WorkspaceFilePage,
} from "@/features/workspace/application/workspaceQueries";
export function visualFilePage(entries: readonly WorkspaceFileEntry[]): WorkspaceFilePage {
  return {
    data: [...entries],
    complete: true,
    coverage: { scannedEntries: entries.length, excludedEntries: 0, budgetExhausted: false },
  };
}
