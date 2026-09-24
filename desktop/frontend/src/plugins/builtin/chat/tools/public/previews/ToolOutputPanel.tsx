import * as stylex from "@stylexjs/stylex";
import { useEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { hasAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import {
  AnsiText,
  Icon,
  IconButton,
  TextButton,
  Well,
  reveal,
  scrollEdges,
  useScrollEdges,
} from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { space } from "@/styles/tokens.stylex";

const op = stylex.create({
  frame: { overflow: "hidden" },
  lines: { fontVariantLigatures: "none" },
  line: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  anchor: { position: "relative" },
  fadeTop: {
    pointerEvents: "none",
    position: "absolute",
    bottom: "calc(var(--spacing) * -6)",
    insetInline: 0,
    height: space.s6,
    backgroundImage: "linear-gradient(to bottom, var(--color-sunken), transparent)",
  },
  more: { justifyContent: "center", paddingBlock: space.s1_5 },
  viewport: { maxHeight: "min(50vh, 20lh)", overflowX: "auto", overflowY: "auto" },
  latest: { position: "absolute", right: space.s2, bottom: space.s2 },
  canvas: { position: "relative", width: "100%" },
  virtualLine: { position: "absolute", top: 0, left: 0, width: "100%" },
});

const COLLAPSED_LINES = 9;
const FOLLOW_SLACK_PX = 4;

function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return <AnsiText text={text} />;
}

function selectionInside(element: HTMLElement | null): boolean {
  const selection = window.getSelection();
  return (
    element !== null &&
    selection !== null &&
    !selection.isCollapsed &&
    selection.anchorNode !== null &&
    element.contains(selection.anchorNode)
  );
}

function ScrollableOutput({ lines }: { lines: string[] }) {
  const t = useT();
  const edges = useScrollEdges();
  const [following, setFollowing] = useState(true);
  const followingRef = useRef(true);
  const rows = useVirtualizer({
    count: lines.length,
    getScrollElement: () => edges.port.current,
    estimateSize: () => 24,
    overscan: 8,
  });

  useEffect(() => {
    if (!followingRef.current || selectionInside(edges.port.current)) return;
    rows.scrollToIndex(lines.length - 1, { align: "end" });
  }, [lines.length, rows, edges.port]);

  const onScroll = (event: React.UIEvent<HTMLDivElement>) => {
    edges.onScroll();
    const port = event.currentTarget;
    const atEnd = port.scrollTop + port.clientHeight >= port.scrollHeight - FOLLOW_SLACK_PX;
    followingRef.current = atEnd;
    setFollowing(atEnd);
  };

  return (
    <div {...stylex.props(op.anchor)}>
      <div
        ref={edges.port}
        role="region"
        aria-label={t("tools.output.label")}
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- The scroll region needs keyboard scrolling independently of the transcript.
        tabIndex={0}
        onScroll={onScroll}
        style={edges.style}
        {...stylex.props(op.lines, op.viewport, scrollEdges.fade)}
      >
        <div
          ref={edges.content}
          {...stylex.props(op.canvas)}
          style={{ height: rows.getTotalSize() }}
        >
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
      {!following && (
        <TextButton
          tone="accent"
          size="sm"
          onClick={() => {
            followingRef.current = true;
            setFollowing(true);
            rows.scrollToIndex(lines.length - 1, { align: "end" });
          }}
          className={stylex.props(op.latest).className}
        >
          {t("tools.output.latest")}
        </TextButton>
      )}
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
  const tail = lines.slice(-COLLAPSED_LINES);

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
    <Well as="div" data-quote-source="tool-output" className={stylex.props(op.frame).className}>
      {hidden > 0 && (
        <div {...stylex.props(op.anchor)}>
          <TextButton
            onClick={() => setExpanded((value) => !value)}
            shape="row"
            size="sm"
            aria-expanded={expanded}
            className={stylex.props(op.more).className}
          >
            <Icon name={expanded ? "chevron-down" : "chevron-up"} size="xs" />
            {expanded
              ? t("tools.output.collapse")
              : t("tools.output.earlier", { count: hidden, total: lines.length })}
          </TextButton>
          {!expanded && <div {...stylex.props(op.fadeTop)} />}
        </div>
      )}
      <div className={cn(stylex.props(reveal.host).className, "relative")}>
        {expanded ? (
          <ScrollableOutput lines={lines} />
        ) : (
          <div {...stylex.props(op.lines)}>
            {tail.map((line, index) => (
              <div key={Math.max(0, hidden) + index} data-output-line="" {...stylex.props(op.line)}>
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
    </Well>
  );
}
