import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import { motion as anim } from "motion/react";
import { createContext, use, useId, useMemo, useState, type ReactNode } from "react";
import { selectionTransition } from "@/lib/motion";
import { radius, surface } from "@/styles/tokens.stylex";

interface HoverTrack {
  layoutId: string;
  hovered: string | null;
  enter: (key: string) => void;
}

const Ctx = createContext<HoverTrack | null>(null);

const host = stylex.create({ boxless: { display: "contents" } });

/**
 * A list where the pointer's highlight is ONE element that travels, rather than a background
 * each row fades in and out on its own.
 *
 * It is the only way to answer the gap between rows: a per-row `:hover` fill goes out as the
 * pointer crosses a 2px gutter and comes back on the next row, so a sweep down the list reads
 * as a flicker. The track holds the last row it was given until another row claims it, and
 * only lets go when the pointer leaves the list — the highlight is never nowhere.
 */
export function HoverTrack({
  children,
  styles,
}: {
  children: ReactNode;
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  const layoutId = useId();
  const [hovered, setHovered] = useState<string | null>(null);
  const track = useMemo<HoverTrack>(
    () => ({ layoutId, hovered, enter: setHovered }),
    [layoutId, hovered],
  );
  return (
    <Ctx value={track}>
      <div {...stylex.props(host.boxless, styles)} onPointerLeave={() => setHovered(null)}>
        {children}
      </div>
    </Ctx>
  );
}

interface HoverTrackItem {
  layoutId: string;
  hovered: boolean;
  onPointerEnter: () => void;
}

export function useHoverTrackItem(): HoverTrackItem | null {
  const track = use(Ctx);
  const key = useId();
  if (!track) return null;
  return {
    layoutId: track.layoutId,
    hovered: track.hovered === key,
    onPointerEnter: () => track.enter(key),
  };
}

const highlight = stylex.create({
  fill: {
    position: "absolute",
    inset: 0,
    zIndex: -1,
    borderRadius: radius.row,
    backgroundColor: surface.hover,
  },
});

/**
 * The travelling fill itself, rendered by whichever item currently holds the pointer. It sits
 * behind the item's own content, so the item has to open a stacking context and stop painting
 * a hover fill of its own.
 */
export function HoverHighlight({
  item,
  styles,
}: {
  item: HoverTrackItem;
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  return (
    <anim.span
      aria-hidden
      layoutId={item.layoutId}
      transition={selectionTransition}
      {...stylex.props(highlight.fill, styles)}
    />
  );
}
