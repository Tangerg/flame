import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { recordedGrepRows } from "@/plugins/builtin/chat/tools/application/toolPreviewRecords";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/agent/public/toolIcon";

import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { gap, TextPreview, vocab } from "@/ui";

const gp = stylex.create({
  head: { display: "flex", alignItems: "baseline", gap: space.s2 },
  raw: { margin: 0 },
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

function GrepPreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const recorded = recordedGrepRows(tool, MAX_GREP_MATCHES);
  if (!recorded) {
    return tool.result ? (
      <TextPreview>
        <pre {...stylex.props(pv.wrap, vocab.muted, typeStep.uiSm, face.mono, gp.raw)}>
          {tool.result}
        </pre>
      </TextPreview>
    ) : null;
  }
  const { shown, overflow } = recorded;
  return (
    <TextPreview>
      <div {...stylex.props(vocab.column, gap.s1_5)}>
        {groupByFile(shown).map((group) => (
          <div key={group.file}>
            <div {...stylex.props(gp.head)}>
              <span
                {...stylex.props(vocab.fill, vocab.truncate, vocab.soft, typeStep.uiSm, face.mono)}
              >
                <LinkedText text={group.file} />
              </span>
              {group.matches.length > 1 && (
                <span {...stylex.props(vocab.hold, vocab.faint, typeStep.ui2xs, face.mono)}>
                  {t("tools.grep.matchCount", { count: group.matches.length })}
                </span>
              )}
            </div>
            {group.matches.map((match, index) => (
              <div key={index} {...stylex.props(pv.numbered, pv.numberedWide, pv.row)}>
                <span {...stylex.props(pv.gutter, typeStep.ui2xs, face.mono)}>{match.line}</span>
                <span {...stylex.props(pv.wrap, vocab.muted, typeStep.uiSm, face.mono)}>
                  {match.text}
                </span>
              </div>
            ))}
          </div>
        ))}
        {overflow > 0 && (
          <div {...stylex.props(vocab.faint)}>
            … {t("tools.overflow.matches", { count: overflow })}
          </div>
        )}
      </div>
    </TextPreview>
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
