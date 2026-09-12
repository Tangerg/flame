import * as stylex from "@stylexjs/stylex";
import { useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
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
  fade: {
    pointerEvents: "none",
    position: "absolute",
    top: "calc(var(--spacing) * -6)",
    insetInline: 0,
    height: space.s6,
    backgroundImage: "linear-gradient(to top, var(--color-sunken), transparent)",
  },
  more: { justifyContent: "center", paddingBlock: space.s1_5 },
  viewport: { height: "min(50vh, 20lh)", overflowY: "auto" },
  canvas: { position: "relative", width: "100%" },
  virtualLine: { position: "absolute", top: 0, left: 0, width: "100%" },
});

const COLLAPSED_LINES = 9;
// Large output stays fully accessible without mounting every line at once.
const VIRTUALIZE_AFTER_LINES = 1_000;

// Plain lines go through `LinkedText`, which turns a path into somewhere to click. A line
// carrying escape codes does not: the link scanner would have to be taught the codes, and a
// coloured `go test` line is the one shape where the path is already the least of what is
// there.
function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return <AnsiText text={text} />;
}

function ScrollableOutput({ lines }: { lines: string[] }) {
  const t = useT();
  const scrollRef = useRef<HTMLDivElement>(null);
  const rows = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 24,
    overscan: 8,
  });
  return (
    <div
      ref={scrollRef}
      role="region"
      aria-label={t("tools.output.label")}
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- The scroll region needs keyboard scrolling independently of the transcript.
      tabIndex={0}
      {...stylex.props(op.sheet, op.viewport, typeStep.code)}
    >
      <div {...stylex.props(op.canvas)} style={{ height: rows.getTotalSize() }}>
        {rows.getVirtualItems().map((row) => (
          <div
            key={row.key}
            data-index={row.index}
            data-output-line=""
            ref={rows.measureElement}
            {...stylex.props(op.line, op.virtualLine)}
            style={{ transform: `translateY(${row.start}px)` }}
          >
            <OutputLine text={lines[row.index]!} />
          </div>
        ))}
      </div>
    </div>
  );
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
  const shown = expanded ? lines : lines.slice(0, COLLAPSED_LINES);

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
        {expanded && lines.length > VIRTUALIZE_AFTER_LINES ? (
          <ScrollableOutput lines={lines} />
        ) : (
          <div {...stylex.props(op.sheet, typeStep.code)}>
            {shown.map((line, index) => (
              <div key={index} data-output-line="" {...stylex.props(op.line)}>
                <OutputLine text={line} />
              </div>
            ))}
          </div>
        )}
        <IconButton
          data-reveal="hover"
          icon={copied ? "check" : "copy"}
          size="xs"
          title={t(copied ? "tools.output.copied" : "tools.output.copy")}
          onClick={() => void copy()}
          className={stylex.props(reveal.shown).className}
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
              : t("tools.output.showAll", { count: lines.length })}
          </TextButton>
        </div>
      )}
    </div>
  );
}
