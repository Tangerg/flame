import * as stylex from "@stylexjs/stylex";
import type { BlockStatus } from "@/plugins/sdk/types/contentBlock";
import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Icon, Loader, vocab } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { face, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { messageStyles as ms } from "../messageStyles";

const FADE = "24px";

const rb = stylex.create({
  note: { marginTop: space.s1 },
  /** The reasoning reads as an aside: indented past the glyph, with a rule marking its extent. */
  aside: {
    marginLeft: space.s5,
    borderLeftWidth: "1px",
    borderLeftStyle: "solid",
    borderLeftColor: surface.field,
    paddingTop: space.s0_5,
    paddingBottom: space.s1_5,
    paddingLeft: space.s6,
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
  /** While it streams, the reasoning is a window onto a growing text rather than the whole of it. */
  windowed: { maxHeight: "calc(var(--spacing) * 48)", overflowY: "auto" },
  /**
   * The clipped edges fade out, as a MASK on the scroller.
   *
   * Two absolutely-positioned gradient overlays used to do this, and neither could ever be
   * seen: `position: absolute; top: 0` inside `overflow-y: auto` anchors to the SCROLLED
   * content origin, so the top fade scrolled out of view exactly when `edges.scrolled` turned
   * it on — measured at -200px after a 200px scroll. The bottom one sat at the end of the
   * text, which is below the viewport whenever `!atBottom` said to show it.
   *
   * A mask is painted against the element's own box and does not scroll, so it lands where the
   * clipping actually happens. It also stops needing to know what is behind it: the overlays
   * hard-coded `--app-content-surface` as their opaque end, which would have painted the wrong
   * colour the moment this block sat on any other surface. `truncate-fade` in `globals.css` is
   * the horizontal sibling of this and already worked this way.
   */
  fade: {
    maskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
    WebkitMaskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
  },
});

interface Props {
  text: string;
  status: BlockStatus;
  superseded?: boolean;
}

export function ReasoningBlock({ text, status, superseded = false }: Props) {
  const t = useT();
  const streaming = status === "running";
  const [openOverride, setOpenOverride] = useState<boolean | null>(null);
  const isOpen = openOverride ?? (streaming && !superseded);

  const toggle = () => {
    setOpenOverride(!isOpen);
  };

  const label = streaming ? t("reasoning.thinking") : t("reasoning.thought");

  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ scrolled: false, atBottom: true, overflowing: false });
  const measure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const next = {
      scrolled: el.scrollTop > 0,
      atBottom: el.scrollHeight - el.scrollTop - el.clientHeight < 4,
      overflowing: el.scrollHeight > el.clientHeight,
    };
    setEdges((prev) =>
      prev.scrolled === next.scrolled &&
      prev.atBottom === next.atBottom &&
      prev.overflowing === next.overflowing
        ? prev
        : next,
    );
  }, []);

  useEffect(() => {
    if (!streaming) return;
    const scrollEl = scrollRef.current;
    const contentEl = contentRef.current;
    if (!scrollEl || !contentEl) return;
    const pin = () => {
      const distanceFromBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
      if (distanceFromBottom < 4) {
        scrollEl.scrollTop = scrollEl.scrollHeight;
      }
      measure();
    };
    pin();
    const ro = new ResizeObserver(pin);
    ro.observe(contentEl);
    return () => ro.disconnect();
  }, [streaming, measure]);

  useEffect(() => {
    measure();
  }, [text, isOpen, measure]);

  const showTopFade = isOpen && edges.scrolled;
  const showBottomFade = isOpen && streaming && edges.overflowing && !edges.atBottom;

  return (
    <AgentActivityDisclosure
      icon="sparkle"
      shell="line"
      label={streaming ? <Loader size="sm" text={label} /> : label}
      toggleLabel={label}
      open={isOpen}
      onToggle={toggle}
      contentClassName={stylex.props(rb.aside).className}
    >
      <div
        ref={scrollRef}
        data-slot="reasoning-scroller"
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={streaming && isOpen ? 0 : undefined}
        onScroll={measure}
        style={
          {
            "--fade-top": showTopFade ? FADE : "0px",
            "--fade-bottom": showBottomFade ? FADE : "0px",
          } as CSSProperties
        }
        {...stylex.props(rb.scroller, rb.fade, streaming && isOpen && rb.windowed)}
      >
        <div ref={contentRef} className={stylex.props(ms.quote, typeStep.uiSm).className}>
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
