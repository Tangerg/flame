import { patchToolResult } from "@/plugins/sdk";
import type { ChangeStatus } from "@flame/runtime-contract/wire";

export interface PatchChange {
  path: string;
  status: ChangeStatus;
  from?: string;
}

const PATCH_STATUSES: ReadonlySet<string> = new Set<ChangeStatus>([
  "added",
  "deleted",
  "modified",
  "moved",
]);

/**
 * The persisted result of one `apply_patch` ToolCall.
 *
 * Shared Agent language because the central Narrative and the right-side Run Summary read
 * the same durable receipt. An entry the contract does not describe is dropped, not the
 * whole receipt; callers must not substitute current worktree state, which belongs to a
 * different scope and point in time.
 */
export function projectPatchChanges(result: string | undefined): PatchChange[] {
  const changes = patchToolResult(result)?.changes;
  if (!Array.isArray(changes)) return [];
  return changes.flatMap((change): PatchChange[] => {
    if (!change?.path || !PATCH_STATUSES.has(change.status)) return [];
    return [
      {
        path: change.path,
        status: change.status,
        ...(change.status === "moved" && change.from ? { from: change.from } : {}),
      },
    ];
  });
}
