import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useGrepToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewQueries";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/chat/tools/public/toolIcon";

import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";
import { previewStyles as pv } from "./previewStyles";
import { TEXT_PREVIEW } from "./previewChrome";

const gp = stylex.create({
  head: { display: "flex", alignItems: "baseline", gap: space.s2 },
});

const MAX_GREP_MATCHES = 4;

function groupByFile(rows: readonly { loc: string; text: string }[]) {
  const groups: { file: string; matches: { line: string; text: string }[] }[] = [];
  for (const row of rows) {
    const cut = row.loc.lastIndexOf(":");
    const file = cut > 0 ? row.loc.slice(0, cut) : row.loc;
    const line = cut > 0 ? row.loc.slice(cut + 1) : "";
    const last = groups[groups.length - 1];
    if (last?.file === file) last.matches.push({ line, text: row.text });
    else groups.push({ file, matches: [{ line, text: row.text }] });
  }
  return groups;
}

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const { shown, overflow } = useGrepToolPreview(tool, MAX_GREP_MATCHES);
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
      <div {...stylex.props(ct.stackTight)}>
        {groupByFile(shown).map((group) => (
          <div key={group.file}>
            <div {...stylex.props(gp.head)}>
              <span {...stylex.props(ct.fill, ct.truncate, ct.soft, typeStep.uiSm, face.mono)}>
                <LinkedText text={group.file} />
              </span>
              {group.matches.length > 1 && (
                <span {...stylex.props(ct.hold, ct.faint, typeStep.ui2xs, face.mono)}>
                  {t("tools.grep.matchCount", { count: group.matches.length })}
                </span>
              )}
            </div>
            {group.matches.map((match, index) => (
              <div key={index} {...stylex.props(pv.numbered, pv.numberedWide, pv.row)}>
                <span {...stylex.props(pv.gutter, typeStep.ui2xs, face.mono)}>{match.line}</span>
                <span {...stylex.props(pv.wrap, ct.muted, typeStep.uiSm, face.mono)}>
                  {match.text}
                </span>
              </div>
            ))}
          </div>
        ))}
        {overflow > 0 && (
          <div {...stylex.props(ct.faint)}>
            … {t("tools.overflow.matches", { count: overflow })}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.viewMatches" onClick={onOpenView} />
    </div>
  );
}

export const grep = definePlugin({
  name: "flame.builtin.grep",
  setup(ctx) {
    for (const preview of toolPreviews({
      grep: GrepPreview,
      [toolShapeKey("search")]: GrepPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
