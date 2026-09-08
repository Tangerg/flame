import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { toneInk, vocab } from "@/ui";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { TEXT_PREVIEW } from "./previewChrome";

const ls = stylex.create({
  pair: { display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: space.s3 },
});

function LspLocationsPreview({ tool, onOpenView }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
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
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function LspHoverPreview({ tool, onOpenView }: ToolPreviewProps) {
  const text = tool.result?.trim();
  return (
    <div {...stylex.props(TEXT_PREVIEW, pv.wrapWords, vocab.soft)}>
      {text || (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.querying"
          idle="tools.preview.idle.empty"
        />
      )}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// Map, not object: keyed by the first word of a tool's output line, so `constructor` would
// pull an inherited member out and paint its source into the className.
// The severity is a `Tone`, and `toneInk` is the one place that turns one into ink.
const SEVERITY_TONE = new Map<string, Tone>([
  ["error", "negative"],
  ["warning", "warning"],
]);

function LspDiagnosticsPreview({ tool, onOpenView }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
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
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
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
    // `diagnostics` is an OPERATION of `lsp`, not a separate tool, so the preview reads
    // the operation to decide which face to wear.
    for (const preview of toolPreviews({ lsp: LspPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
