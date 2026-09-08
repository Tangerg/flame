import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectSkillPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { Tag } from "@/ui";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";
import { previewStyles as pv } from "./previewStyles";
import { TEXT_PREVIEW } from "./previewChrome";

const sk = stylex.create({
  row: { display: "flex", alignItems: "baseline", gap: space.s2 },
});

function SkillCatalogPreview({ tool, onOpenView }: ToolPreviewProps) {
  const entries = projectSkillPreview(tool.result);
  if (entries.length === 0) {
    return (
      <div {...stylex.props(TEXT_PREVIEW)}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.noTools"
        />
      </div>
    );
  }
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
      {entries.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((s) => (
        <div key={s.name} className={stylex.props(sk.row, pv.row, pv.rowPad).className}>
          <Tag size="sm">{s.name}</Tag>
          <span {...stylex.props(ct.truncate, ct.muted, typeStep.uiSm)}>{s.description}</span>
        </div>
      ))}
      <PreviewOverflow count={entries.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function SkillTextPreview({ tool, onOpenView }: ToolPreviewProps) {
  const lines = resultLines(tool.result);
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
      {lines.length > 0 ? (
        <div {...stylex.props(pv.wrapWords, ct.soft)}>
          {lines.slice(0, INLINE_PREVIEW_ROW_LIMIT).join("\n")}
        </div>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.empty"
        />
      )}
      <PreviewOverflow count={lines.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
    </div>
  );
}

function LoadedSkillPreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

function SkillResourcePreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

function SkillProposalPreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

export const skillPreview = definePlugin({
  name: "flame.builtin.skill-preview",
  setup(ctx) {
    for (const preview of toolPreviews({
      list_skills: SkillCatalogPreview,
      load_skill: LoadedSkillPreview,
      read_skill_resource: SkillResourcePreview,
      propose_skill: SkillProposalPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
