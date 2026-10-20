import * as stylex from "@stylexjs/stylex";
import { type BlockStatus } from "@/features/agent/public/contentBlock";
import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { MarkdownMessage } from "@/features/chat/message/ui/markdown/MarkdownMessage";
import { Icon } from "@/ui/index";
import { Loader } from "@/ui/index";
import { scrollEdges, useScrollEdges } from "@/ui/index";
import { vocab } from "@flame/ui";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { fmtDuration } from "@/lib/format";
import { useT } from "@/lib/i18n/index";
import { face, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { messageStyles as ms } from "@/features/chat/message/ui/messageStyles";
const FOLLOW_SLACK = 24;

const GLIMPSE_LEAD = "16px";

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
  glimpse: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    maskImage: `linear-gradient(to right, transparent 0, #000 var(--glimpse-lead, 0px))`,
    WebkitMaskImage: `linear-gradient(to right, transparent 0, #000 var(--glimpse-lead, 0px))`,
  },
  glimpseTail: { justifyContent: "flex-end" },
  glimpseLine: { flexShrink: 0, whiteSpace: "nowrap" },
  aside: {
    marginTop: space.s3,
    paddingBottom: space.s1_5,
  },
  scroller: {
    position: "relative",
    overflowX: "hidden",
    overflowY: "hidden",
    paddingRight: space.s2,
  },
  windowed: { maxHeight: "calc(var(--spacing) * 60)", overflowY: "auto" },
});

interface Props {
  text: string;
  status: BlockStatus;
}

export function ReasoningBlock({ text, status }: Props) {
  const t = useT();
  const streaming = status === "running";
  const [isOpen, setOpen] = useState(false);
  const toggle = useCallback(() => setOpen((value) => !value), []);

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

  const glimpse = streaming && !isOpen ? currentThought(text) : undefined;

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
      icon="brain"
      shell="line"
      label={streaming ? <Loader text={label} /> : label}
      detail={
        glimpse && (
          <span
            ref={glimpseBoxRef}
            data-slot="reasoning-glimpse"
            data-clipped={glimpseClipped ? "" : undefined}
            aria-hidden
            style={{ "--glimpse-lead": glimpseClipped ? GLIMPSE_LEAD : "0px" } as CSSProperties}
            {...stylex.props(rb.glimpse, glimpseClipped && rb.glimpseTail)}
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
        <div ref={content} className={stylex.props(ms.quote, typeStep.uiMd).className}>
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
