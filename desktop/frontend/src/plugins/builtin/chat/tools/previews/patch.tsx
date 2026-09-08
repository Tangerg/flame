import * as stylex from "@stylexjs/stylex";
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
import { DiffStat, FilePath, vocab } from "@/ui";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { TEXT_PREVIEW } from "./previewChrome";
import { color, leading, space, type as typeStep } from "@/styles/tokens.stylex";

const pt = stylex.create({
  // One track for the verbs, shared by every row through `subgrid`, so every path begins on
  // the same left edge however long its own verb is.
  track: { display: "grid", gridTemplateColumns: "auto minmax(0, 1fr)", columnGap: space.s1_5 },
  changeRow: {
    gridColumn: "span 2",
    display: "grid",
    gridTemplateColumns: "subgrid",
    alignItems: "center",
    paddingBlock: space.s0_5,
    lineHeight: leading.body,
  },
  verb: { fontFamily: "var(--font-sans)", color: color.fgFaint },
  movedLine: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1,
    color: color.fgMuted,
  },
  // The source path yields most of the room: what matters is where the file went.
  fromPath: { maxWidth: "42%" },
  proposedRow: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
    paddingBlock: space.s0_5,
    lineHeight: leading.body,
  },
});

const STATUS_KEY: Record<PatchChange["status"], string> = {
  added: "tools.patch.created",
  deleted: "tools.patch.deleted",
  modified: "tools.patch.edited",
  moved: "tools.patch.moved",
};

function PatchChangeRow({ change }: { change: PatchChange }) {
  const t = useT();
  return (
    <div data-patch-change={change.status} {...stylex.props(pt.changeRow, typeStep.uiMd)}>
      <span {...stylex.props(pt.verb)}>{t(STATUS_KEY[change.status])}</span>
      {change.status === "moved" && change.from ? (
        <span {...stylex.props(pt.movedLine)}>
          <FilePath path={change.from} className={stylex.props(pt.fromPath).className} />
          <span aria-hidden="true" {...stylex.props(vocab.hold, vocab.faint)}>
            →
          </span>
          <FilePath path={change.path} className={stylex.props(vocab.fill).className} />
        </span>
      ) : (
        <FilePath path={change.path} className={stylex.props(vocab.min, vocab.muted).className} />
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
    <div {...stylex.props(pt.proposedRow, typeStep.uiMd)}>
      <FilePath path={change.path} className={stylex.props(vocab.fill, vocab.muted).className} />
      <DiffStat added={change.added} removed={change.removed} />
    </div>
  );
}

export function ApplyPatchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const changes = projectPatchChanges(tool.result);
  const proposed = tool.status === "running" ? (tool.changes ?? []) : [];
  const rows = changes.length > 0 ? changes.length : proposed.length;
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
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
      <div {...stylex.props(pt.track)}>
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
