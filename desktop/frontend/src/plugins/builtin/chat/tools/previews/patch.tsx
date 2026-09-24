import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectPatchChanges, type PatchChange } from "@/plugins/builtin/agent/public/patchResult";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/agent/public/toolIcon";
import type { ToolFileChange } from "@/plugins/sdk/types/agentSessionView";
import { useState } from "react";
import { DiffStat, FilePath, Pressable, TextButton, TextPreview, vocab } from "@/ui";
import { openFileInWorkingTreeDiff } from "@/plugins/builtin/workspace/public/deeplinks";
import { INLINE_PREVIEW_ROW_LIMIT } from "./previewChrome";
import { color, leading, space, type as typeStep } from "@/styles/tokens.stylex";

const pt = stylex.create({
  track: { display: "grid", gridTemplateColumns: "auto minmax(0, 1fr)", columnGap: space.s1_5 },
  changeRow: {
    gridColumn: "span 2",
    display: "grid",
    gridTemplateColumns: "subgrid",
    alignItems: "center",
    borderWidth: 0,
    backgroundColor: "transparent",
    paddingBlock: space.s0_5,
    paddingInline: 0,
    textAlign: "left",
    lineHeight: leading.body,
    color: { default: "inherit", ":hover": color.fg },
  },
  verb: { fontFamily: "var(--font-sans)", color: color.fgFaint },
  movedLine: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1,
    color: color.fgMuted,
  },
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
    <Pressable
      type="button"
      data-patch-change={change.status}
      title={t("tool.open.worktreeDiff")}
      onClick={() => openFileInWorkingTreeDiff(change.path)}
      className={stylex.props(pt.changeRow, typeStep.uiMd).className}
    >
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
    </Pressable>
  );
}

function ProposedChangeRow({ change }: { change: ToolFileChange }) {
  return (
    <div {...stylex.props(pt.proposedRow, typeStep.uiMd)}>
      <FilePath path={change.path} className={stylex.props(vocab.fill, vocab.muted).className} />
      <DiffStat added={change.added} removed={change.removed} />
    </div>
  );
}

export function ApplyPatchPreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const [all, setAll] = useState(false);
  const changes = projectPatchChanges(tool.result);
  const proposed = tool.status === "running" ? (tool.changes ?? []) : [];
  const rows = changes.length > 0 ? changes.length : proposed.length;
  const limit = all ? rows : INLINE_PREVIEW_ROW_LIMIT;
  const hidden = rows - limit;
  return (
    <TextPreview>
      {rows === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.noChanges"
        />
      )}
      <div {...stylex.props(pt.track)}>
        {changes.slice(0, limit).map((change) => (
          <PatchChangeRow
            key={`${change.status}:${change.from ?? ""}:${change.path}`}
            change={change}
          />
        ))}
      </div>
      {proposed.slice(0, limit).map((change) => (
        <ProposedChangeRow key={change.path} change={change} />
      ))}
      {hidden > 0 && (
        <TextButton tone="muted" size="sm" onClick={() => setAll(true)}>
          {t("tools.overflow.more", { count: hidden })}
        </TextButton>
      )}
    </TextPreview>
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
