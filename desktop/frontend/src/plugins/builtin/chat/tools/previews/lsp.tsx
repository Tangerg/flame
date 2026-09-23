import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { TextPreview, toneInk, vocab } from "@/ui";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";

const ls = stylex.create({
  pair: { display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: space.s3 },
});

function LspLocationsPreview({ tool }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <TextPreview>
      {rows.length === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.querying"
          idle="tools.preview.idle.empty"
        />
      )}
      {rows.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((row, i) => {
        const sep = row.lastIndexOf(" — ");
        if (sep === -1) {
          return (
            <div
              key={i}
              className={stylex.props(vocab.truncate, pv.row, pv.rowPad, vocab.soft).className}
            >
              {row}
            </div>
          );
        }
        return (
          <div key={i} className={stylex.props(ls.pair, pv.row, pv.rowPad).className}>
            <span {...stylex.props(vocab.truncate, vocab.soft)}>{row.slice(0, sep)}</span>
            <span {...stylex.props(vocab.truncate, vocab.muted, typeStep.uiSm)}>
              {row.slice(sep + 3)}
            </span>
          </div>
        );
      })}
      <PreviewOverflow count={rows.length - INLINE_PREVIEW_ROW_LIMIT} />
    </TextPreview>
  );
}

function LspHoverPreview({ tool }: ToolPreviewProps) {
  const text = tool.result?.trim();
  return (
    <TextPreview wrap="words" ink="soft">
      {text || (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.querying"
          idle="tools.preview.idle.empty"
        />
      )}
    </TextPreview>
  );
}

const SEVERITY_TONE = new Map<string, Tone>([
  ["error", "negative"],
  ["warning", "warning"],
]);

function LspDiagnosticsPreview({ tool }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <TextPreview>
      {rows.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((row, i) => {
        const space = row.indexOf(" ");
        const severity = space === -1 ? "" : row.slice(0, space);
        const tone = SEVERITY_TONE.get(severity);
        if (!tone) {
          return (
            <div key={i} {...stylex.props(vocab.truncate, pv.rowPad, vocab.soft)}>
              {row}
            </div>
          );
        }
        return (
          <div key={i} {...stylex.props(vocab.truncate, pv.rowPad, vocab.soft)}>
            <span {...stylex.props(vocab.strong, toneInk[tone])}>{severity}</span>
            {row.slice(space)}
          </div>
        );
      })}
      <PreviewOverflow count={rows.length - INLINE_PREVIEW_ROW_LIMIT} />
    </TextPreview>
  );
}

function LspPreview(props: ToolPreviewProps) {
  if (props.tool.operation === "hover") return <LspHoverPreview {...props} />;
  if (props.tool.operation === "diagnostics") return <LspDiagnosticsPreview {...props} />;
  return <LspLocationsPreview {...props} />;
}

export const lspPreviews = definePlugin({
  name: "flame.builtin.lsp-previews",
  setup(ctx) {
    for (const preview of toolPreviews({ lsp: LspPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
