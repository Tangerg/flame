import { patchToolResult } from "@/plugins/sdk";
import { WIRE_ENUMS, type ChangeStatus } from "@flame/runtime-contract/wire";

export interface PatchChange {
  path: string;
  status: ChangeStatus;
  from?: string;
}

const PATCH_STATUSES: ReadonlySet<string> = new Set<ChangeStatus>(WIRE_ENUMS.ChangeStatus);

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
