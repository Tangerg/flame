import * as stylex from "@stylexjs/stylex";
import { useEffect, useEffectEvent, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { hasAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import {
  AnsiText,
  Icon,
  IconButton,
  SearchField,
  TextButton,
  Well,
  reveal,
  scrollEdges,
  useScrollEdges,
} from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { color, space, surface, type as typeStep } from "@/styles/tokens.stylex";

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
  match: { backgroundColor: surface.hover },
  currentMatch: { backgroundColor: surface.selected, boxShadow: `inset 2px 0 0 ${color.accent}` },
  actions: {
    position: "absolute",
    top: space.s1,
    right: space.s1,
    display: "flex",
    gap: space.s0_5,
  },
  findBar: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    paddingInline: space.s1,
    paddingBottom: space.s1,
  },
  findField: { flex: 1, minWidth: 0 },
  findCount: { flexShrink: 0, color: color.fgMuted, fontVariantNumeric: "tabular-nums" },
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

function matchingLines(lines: readonly string[], query: string): number[] {
  const needle = query.toLowerCase();
  if (needle === "") return [];
  const out: number[] = [];
  lines.forEach((line, index) => {
    if (line.toLowerCase().includes(needle)) out.push(index);
  });
  return out;
}

interface Find {
  matches: ReadonlySet<number>;
  current: number | null;
}

function FindBar({
  lines,
  onFind,
  onClose,
}: {
  lines: readonly string[];
  onFind: (find: Find) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [position, setPosition] = useState(0);
  const field = useRef<HTMLInputElement>(null);
  useEffect(() => field.current?.focus(), []);
  const matches = useMemo(() => matchingLines(lines, query), [lines, query]);
  const index = matches.length === 0 ? -1 : Math.min(position, matches.length - 1);
  const report = useEffectEvent(onFind);
  useEffect(() => {
    report({ matches: new Set(matches), current: index === -1 ? null : matches[index]! });
  }, [matches, index]);

  const step = (direction: 1 | -1) => {
    if (matches.length === 0) return;
    setPosition((index + direction + matches.length) % matches.length);
  };

  return (
    <div {...stylex.props(op.findBar)}>
      <SearchField
        size="sm"
        font="mono"
        ref={field}
        aria-label={t("tools.output.find")}
        placeholder={t("tools.output.find")}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setPosition(0);
        }}
        onClear={() => setQuery("")}
        clearLabel={t("common.clear")}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Enter") {
            event.preventDefault();
            step(event.shiftKey ? -1 : 1);
          }
          if (event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            onClose();
          }
        }}
        className={stylex.props(op.findField).className}
      />
      <span aria-live="polite" {...stylex.props(op.findCount, typeStep.uiXs)}>
        {query === ""
          ? ""
          : matches.length === 0
            ? t("tools.output.find.none")
            : t("tools.output.find.count", { index: index + 1, total: matches.length })}
      </span>
      <IconButton
        icon="chevron-up"
        size="xs"
        title={t("tools.output.find.previous")}
        disabled={matches.length === 0}
        onClick={() => step(-1)}
      />
      <IconButton
        icon="chevron-down"
        size="xs"
        title={t("tools.output.find.next")}
        disabled={matches.length === 0}
        onClick={() => step(1)}
      />
      <IconButton icon="x" size="xs" title={t("tools.output.find.close")} onClick={onClose} />
    </div>
  );
}

function ScrollableOutput({
  lines,
  find,
  onFindShortcut,
}: {
  lines: string[];
  find: Find | null;
  onFindShortcut: () => void;
}) {
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

  const current = find?.current ?? null;
  // Rows are measured as they render, so a far match drifts while the rows around
  // it settle. The match stays aimed at until the user scrolls, clicks or types.
  const pendingJump = useRef<number | null>(null);

  const stickToEnd = useEffectEvent(() => {
    const port = edges.port.current;
    if (!port || !followingRef.current || selectionInside(port)) return;
    port.scrollTop = port.scrollHeight;
  });
  useLayoutEffect(() => stickToEnd(), [lines.length]);

  const aimAtMatch = useEffectEvent(() => {
    const port = edges.port.current;
    const target = pendingJump.current;
    if (!port || target === null) return;
    const row = port.querySelector<HTMLElement>(`[data-index="${target}"]`);
    if (!row) return;
    const box = row.getBoundingClientRect();
    const view = port.getBoundingClientRect();
    if (box.top < view.top || box.bottom > view.bottom) {
      row.scrollIntoView({ block: "center", inline: "nearest" });
    }
  });
  useLayoutEffect(() => aimAtMatch());

  useEffect(() => {
    const content = edges.content.current;
    if (!content) return;
    const observer = new ResizeObserver(() => {
      stickToEnd();
      aimAtMatch();
    });
    observer.observe(content);
    return () => observer.disconnect();
  }, [edges.content]);

  useEffect(() => {
    const port = edges.port.current;
    pendingJump.current = current;
    if (current === null || !port) return;
    followingRef.current = false;
    setFollowing(false);
    const [offset] = rows.getOffsetForIndex(current, "center") ?? [port.scrollTop];
    port.scrollTop = offset;
  }, [current, rows, edges.port]);

  const releaseJump = () => {
    pendingJump.current = null;
  };

  const onScroll = (event: React.UIEvent<HTMLDivElement>) => {
    edges.onScroll();
    const port = event.currentTarget;
    const atEnd = port.scrollTop + port.clientHeight >= port.scrollHeight - FOLLOW_SLACK_PX;
    followingRef.current = atEnd;
    setFollowing(atEnd);
  };

  return (
    <div {...stylex.props(op.anchor)}>
      {/* oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- The focused scroll region owns the find shortcut for the text it scrolls. */}
      <div
        ref={edges.port}
        role="region"
        aria-label={t("tools.output.label")}
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- The scroll region needs keyboard scrolling independently of the transcript.
        tabIndex={0}
        onScroll={onScroll}
        onWheel={releaseJump}
        onPointerDown={releaseJump}
        onKeyDown={(event) => {
          releaseJump();
          if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "f") {
            event.preventDefault();
            onFindShortcut();
          }
        }}
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
              data-current-match={row.index === current ? "" : undefined}
              {...stylex.props(
                op.line,
                op.virtualLine,
                find?.matches.has(row.index) && op.match,
                row.index === current && op.currentMatch,
              )}
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
            releaseJump();
            followingRef.current = true;
            setFollowing(true);
            const port = edges.port.current;
            if (port) port.scrollTop = port.scrollHeight;
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
  const [finding, setFinding] = useState(false);
  const [find, setFind] = useState<Find | null>(null);
  const closeFind = () => {
    setFinding(false);
    setFind(null);
  };
  const openFind = () => {
    setExpanded(true);
    setFinding(true);
  };

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
            onClick={() => {
              if (expanded) closeFind();
              setExpanded(!expanded);
            }}
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
      {finding && <FindBar lines={lines} onFind={setFind} onClose={closeFind} />}
      <div className={cn(stylex.props(reveal.host).className, "relative")}>
        {expanded || finding ? (
          <ScrollableOutput lines={lines} find={find} onFindShortcut={openFind} />
        ) : (
          <div {...stylex.props(op.lines)}>
            {tail.map((line, index) => (
              <div key={Math.max(0, hidden) + index} data-output-line="" {...stylex.props(op.line)}>
                <OutputLine text={line} />
              </div>
            ))}
          </div>
        )}
        <div data-reveal="hover" {...stylex.props(op.actions, reveal.shown)}>
          {!finding && (
            <IconButton icon="search" size="xs" title={t("tools.output.find")} onClick={openFind} />
          )}
          <IconButton
            icon={copied ? "check" : "copy"}
            size="xs"
            title={t(copied ? "tools.output.copied" : "tools.output.copy")}
            onClick={() => void copy()}
          />
        </div>
      </div>
    </Well>
  );
}
