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
