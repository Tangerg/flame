import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectPatchChanges, type PatchChange } from "@/plugins/builtin/agent/public/patchResult";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/chat/tools/public/toolIcon";
import type { ToolFileChange } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, FilePath } from "@/ui";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

const STATUS_KEY: Record<PatchChange["status"], string> = {
  added: "tools.patch.created",
  deleted: "tools.patch.deleted",
  modified: "tools.patch.edited",
  moved: "tools.patch.moved",
};

function PatchChangeRow({ change }: { change: PatchChange }) {
  const t = useT();
  return (
    <div
      data-patch-change={change.status}
      className="col-span-2 grid grid-cols-subgrid items-center py-0.5 text-ui-md leading-body"
    >
      <span className="font-sans text-fg-faint">{t(STATUS_KEY[change.status])}</span>
      {change.status === "moved" && change.from ? (
        <span className="flex min-w-0 items-center gap-1 text-fg-muted">
          <FilePath path={change.from} className="max-w-[42%]" />
          <span aria-hidden="true" className="shrink-0 text-fg-faint">
            →
          </span>
          <FilePath path={change.path} className="min-w-0 flex-1" />
        </span>
      ) : (
        <FilePath path={change.path} className="min-w-0 text-fg-muted" />
      )}
    </div>
  );
}

/**
 * The files a still-running call is working through, read off the patch it was given.
 *
 * Deliberately without the receipt's verbs: those report what HAPPENED, and this call has
 * not happened yet. Line counts carry the row instead — the receipt never has them.
 */
function ProposedChangeRow({ change }: { change: ToolFileChange }) {
  return (
    <div className="flex min-w-0 items-center gap-1.5 py-0.5 text-ui-md leading-body">
      <FilePath path={change.path} className="min-w-0 flex-1 text-fg-muted" />
      <DiffStat added={change.added} removed={change.removed} />
    </div>
  );
}

export function ApplyPatchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const changes = projectPatchChanges(tool.result);
  const proposed = tool.status === "running" ? (tool.changes ?? []) : [];
  const rows = changes.length > 0 ? changes.length : proposed.length;
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {rows === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.noChanges"
        />
      )}
      {/* One track for the verbs, shared by every row through `subgrid`. The verb used to be a
          `shrink-0` inline label, so each path began wherever its own verb ended — invisible
          while a receipt had one row, a ragged left edge as soon as a patch edits, moves and
          deletes in one call. `auto` means a single-row receipt is still exactly as wide as its
          own verb, and no locale needs a width picked for it. */}
      <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-1.5">
        {changes.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((change) => (
          <PatchChangeRow
            key={`${change.status}:${change.from ?? ""}:${change.path}`}
            change={change}
          />
        ))}
      </div>
      {proposed.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((change) => (
        <ProposedChangeRow key={change.path} change={change} />
      ))}
      <PreviewOverflow count={rows - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

export const applyPatchPreview = definePlugin({
  name: "flame.builtin.apply-patch-preview",
  setup(ctx) {
    for (const preview of toolPreviews({
      apply_patch: ApplyPatchPreview,
      [toolShapeKey("patch")]: ApplyPatchPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
