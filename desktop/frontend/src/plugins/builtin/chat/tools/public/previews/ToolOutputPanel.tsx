import * as stylex from "@stylexjs/stylex";
import { useMemo, useState } from "react";
import { hasAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import { AnsiText, Icon, IconButton, TextButton, Well, reveal } from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { color, leading, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";

const op = stylex.create({
  panel: { overflow: "hidden", borderRadius: radius.sm, backgroundColor: surface.sunken },
  // Output scrolls sideways rather than wrapping: a column of a table is a column.
  sheet: {
    overflowX: "auto",
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
    color: color.fgSoft,
    fontVariantLigatures: "none",
  },
  line: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  anchor: { position: "relative" },
  // A gradient over the last collapsed line, so a cut looks cut rather than ended.
  fade: {
    pointerEvents: "none",
    position: "absolute",
    top: "calc(var(--spacing) * -6)",
    insetInline: 0,
    height: space.s6,
    backgroundImage: "linear-gradient(to top, var(--color-sunken), transparent)",
  },
  more: { justifyContent: "center", paddingBlock: space.s1_5 },
  note: {
    paddingInline: space.s3,
    paddingBottom: space.s2,
    textAlign: "center",
    color: color.fgFaint,
  },
});

const COLLAPSED_LINES = 9;
// Expanding used to render every line there was, and the cost is superlinear: measured at
// 120ms for a thousand lines, 700ms for ten thousand and over nine seconds for fifty
// thousand, which a `shell` running a build reaches without trying. The whole output is a
// click away in the terminal view either way, so inline expansion stops where it is still
// a frame rather than a freeze.
const EXPANDED_LINES = 1_000;

// Plain lines go through `LinkedText`, which turns a path into somewhere to click. A line
// carrying escape codes does not: the link scanner would have to be taught the codes, and a
// coloured `go test` line is the one shape where the path is already the least of what is
// there.
function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return <AnsiText text={text} />;
}

interface ToolOutputPanelProps {
  output: string | undefined;
  status: ToolCall["status"];
  idleLabel?: string;
}

export function ToolOutputPanel({
  output,
  status,
  idleLabel = "tools.preview.idle.noOutput",
}: ToolOutputPanelProps) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);

  const lines = useMemo(() => {
    const trimmed = output?.replace(/\n+$/, "") ?? "";
    return trimmed === "" ? [] : trimmed.split("\n");
  }, [output]);
  const copyMaterial = lines.join("\n");
  const { copied, copy } = useCopyFeedback(copyMaterial);

  const hidden = lines.length - COLLAPSED_LINES;
  const shown = lines.slice(0, expanded ? EXPANDED_LINES : COLLAPSED_LINES);
  const beyond = lines.length - shown.length;

  if (lines.length === 0) {
    return (
      <Well as="div">
        <PreviewPlaceholder
          status={status}
          pending="tools.preview.pending.running"
          idle={idleLabel}
        />
      </Well>
    );
  }

  return (
    <div {...stylex.props(op.panel)}>
      <div className={cn(stylex.props(reveal.host).className, "relative")}>
        <div {...stylex.props(op.sheet, typeStep.code)}>
          {shown.map((line, index) => (
            <div key={index} data-output-line="" {...stylex.props(op.line)}>
              <OutputLine text={line} />
            </div>
          ))}
        </div>
        <IconButton
          data-reveal="hover"
          icon={copied ? "check" : "copy"}
          size="xs"
          title={t(copied ? "tools.output.copied" : "tools.output.copy")}
          onClick={() => void copy()}
          className={cn(undefined, stylex.props(reveal.shown).className)}
        />
      </div>
      {hidden > 0 && (
        <div {...stylex.props(op.anchor)}>
          {!expanded && <div {...stylex.props(op.fade)} />}
          <TextButton
            onClick={() => setExpanded((value) => !value)}
            shape="row"
            size="sm"
            className={stylex.props(op.more).className}
          >
            <Icon name={expanded ? "chevron-up" : "chevron-down"} size="xs" />
            {expanded
              ? t("tools.output.collapse")
              : lines.length > EXPANDED_LINES
                ? t("tools.output.showSome", { count: EXPANDED_LINES, total: lines.length })
                : t("tools.output.showAll", { count: lines.length })}
          </TextButton>
          {expanded && beyond > 0 && (
            <div {...stylex.props(op.note, typeStep.uiSm)}>
              {t("tools.output.beyond", { count: beyond })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
