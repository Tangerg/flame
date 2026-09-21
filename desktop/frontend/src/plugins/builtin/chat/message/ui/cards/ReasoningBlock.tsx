import * as stylex from "@stylexjs/stylex";
import type { BlockStatus } from "@/plugins/sdk/types/contentBlock";
import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Icon, Loader, scrollEdges, useScrollEdges, vocab } from "@/ui";
import { AgentActivityDisclosure, useActivityOpenState } from "@/ui/agent";
import { fmtDuration } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { face, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { messageStyles as ms } from "../messageStyles";

const FOLLOW_SLACK = 24;
const GLIMPSE_LEAD = "24px";

/** The last line the model has written. Empty while it is between paragraphs. */
function currentThought(text: string): string | undefined {
  const lines = text.split("\n");
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index]?.trim();
    if (line) return line;
  }
  return undefined;
}

const rb = stylex.create({
  note: { marginTop: space.s1 },
  /**
   * The NEWEST words, which is why the line is pinned to its end rather than truncated at it.
   * A paragraph's opening does not change while the model is still writing it, so a glimpse
   * anchored the usual way would sit still for a minute and report nothing.
   *
   * A non-shrinking child in a flex box that packs to the end spills over its start edge, where
   * the mask takes it. The lead is zero until it has something to soften: this used to be a
   * constant on the theory that a line short enough to fit leaves the start edge empty, which is
   * only true with room to spare. `flex-end` puts the line's first pixel at `container - line`,
   * so any line within the lead of filling the row starts INSIDE the ramp — measured 13.3px in
   * at 656px of 670, and 1.9px in at 668. The line grows a token at a time, so every one of them
   * crosses that window on its way to overflowing and ghosts its own opening on the way through.
   */
  glimpse: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    justifyContent: "flex-end",
    overflow: "hidden",
    maskImage: `linear-gradient(to right, transparent 0, #000 var(--glimpse-lead, 0px))`,
    WebkitMaskImage: `linear-gradient(to right, transparent 0, #000 var(--glimpse-lead, 0px))`,
  },
  glimpseLine: { flexShrink: 0, whiteSpace: "nowrap" },
  // Two alignments the row already decides, rather than none.
  //
  // The rail hangs from the centre of the 16px mark, and the prose lands where the LABEL starts
  // — mark plus the trigger's own gap. It used to sit 20px in with 24px of padding, which put
  // the rail two pixels left of the label (a near-miss reads as a mistake) and the prose
  // twenty-three past it, aligned with nothing and spending that much of the reading measure.
  aside: {
    marginLeft: space.s2,
    borderLeftWidth: "var(--control-edge-width)",
    borderLeftStyle: "solid",
    borderLeftColor: surface.field,
    paddingTop: space.s0_5,
    paddingBottom: space.s1_5,
    paddingLeft: space.s3_5,
  },
  // Both axes as LONGHANDS, because `windowed` below reopens one of them. `stylex.props()`
  // resolves precedence between styles that name the same KEY; `overflow` and `overflowY` are
  // two keys, so it emits both and the winner becomes whichever rule the bundler wrote second
  // — which is not a decision this file gets to make by argument order.
  scroller: {
    position: "relative",
    overflowX: "hidden",
    overflowY: "hidden",
    paddingRight: space.s2,
  },
  windowed: { maxHeight: "calc(var(--spacing) * 48)", overflowY: "auto" },
});

interface Props {
  text: string;
  status: BlockStatus;
  superseded?: boolean;
}

export function ReasoningBlock({ text, status, superseded = false }: Props) {
  const t = useT();
  const streaming = status === "running";
  const { open: isOpen, toggle } = useActivityOpenState(streaming && !superseded);

  // The Runtime publishes a reasoning stream, not a duration; this is the wait as lived here.
  // A restored session never saw the run, so the bare word stands.
  const startedAt = useRef<number | null>(null);
  const [thoughtMillis, setThoughtMillis] = useState<number | null>(null);
  useEffect(() => {
    if (streaming) {
      startedAt.current ??= performance.now();
      return;
    }
    const started = startedAt.current;
    if (started === null) return;
    startedAt.current = null;
    setThoughtMillis(performance.now() - started);
  }, [streaming]);

  const label = streaming
    ? t("reasoning.thinking")
    : thoughtMillis === null
      ? t("reasoning.thought")
      : t("reasoning.thoughtFor", { duration: fmtDuration(thoughtMillis) });

  // Only while it is BOTH running and shut: open, the material itself is on screen, and the row
  // would be repeating its own first line back at the reader.
  const glimpse = streaming && !isOpen ? currentThought(text) : undefined;

  // Whether the line is actually being cut, which is the only time the lead has anything to
  // soften. Observed rather than derived from `text`: the width changes on a token that adds no
  // line, and a resize moves the boundary with no new token at all.
  const glimpseBoxRef = useRef<HTMLSpanElement>(null);
  const glimpseLineRef = useRef<HTMLSpanElement>(null);
  const [glimpseClipped, setGlimpseClipped] = useState(false);
  useEffect(() => {
    const box = glimpseBoxRef.current;
    const line = glimpseLineRef.current;
    if (!box || !line) return;
    const read = () => setGlimpseClipped(line.offsetWidth > box.clientWidth);
    read();
    const ro = new ResizeObserver(read);
    ro.observe(box);
    ro.observe(line);
    return () => ro.disconnect();
  }, [glimpse]);

  const {
    port,
    content,
    onScroll: measureEdges,
    style: edgeStyle,
    overflowing,
    distanceFromEnd,
    scrollToEnd,
  } = useScrollEdges(isOpen);

  // Follows the newest line, and stops the moment the reader scrolls away from it.
  const followingRef = useRef(true);
  const onScroll = useCallback(() => {
    followingRef.current = distanceFromEnd() < FOLLOW_SLACK;
    measureEdges();
  }, [distanceFromEnd, measureEdges]);

  useEffect(() => {
    if (!streaming || !isOpen) return;
    const contentEl = content.current;
    if (!contentEl) return;
    const follow = () => {
      if (followingRef.current) scrollToEnd();
    };
    follow();
    const ro = new ResizeObserver(follow);
    ro.observe(contentEl);
    return () => ro.disconnect();
  }, [streaming, isOpen, content, scrollToEnd]);

  return (
    <AgentActivityDisclosure
      icon="sparkle"
      shell="line"
      label={streaming ? <Loader text={label} /> : label}
      detail={
        glimpse && (
          // Not announced: `RunAnnouncer` owns what the run is doing, and a line that changes
          // on every token would talk over it.
          <span
            ref={glimpseBoxRef}
            data-slot="reasoning-glimpse"
            data-clipped={glimpseClipped ? "" : undefined}
            aria-hidden
            style={{ "--glimpse-lead": glimpseClipped ? GLIMPSE_LEAD : "0px" } as CSSProperties}
            {...stylex.props(rb.glimpse)}
          >
            <span ref={glimpseLineRef} {...stylex.props(rb.glimpseLine, vocab.faint)}>
              {glimpse}
            </span>
          </span>
        )
      }
      toggleLabel={label}
      open={isOpen}
      onToggle={toggle}
      contentClassName={stylex.props(rb.aside).className}
    >
      <div
        ref={port}
        data-slot="reasoning-scroller"
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={overflowing ? 0 : undefined}
        onScroll={onScroll}
        style={edgeStyle}
        {...stylex.props(rb.scroller, scrollEdges.fade, isOpen && rb.windowed)}
      >
        <div ref={content} className={stylex.props(ms.quote, typeStep.uiSm).className}>
          <MarkdownMessage text={text} streaming={streaming} reveal="smooth" />
          {status === "incomplete" && (
            <div {...stylex.props(rb.note, vocab.faint, typeStep.uiSm, face.mono)}>
              <Icon name="x" size="xs" /> {t("reasoning.interrupted")}
            </div>
          )}
        </div>
      </div>
    </AgentActivityDisclosure>
  );
}
