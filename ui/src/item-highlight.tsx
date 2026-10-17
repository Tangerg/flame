import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import { motion as anim, useReducedMotion } from "motion/react";
import {
  createContext,
  use,
  useId,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  reducedSelectionTransition,
  selectionTransition,
} from "./selection-motion";
import { radius, surface } from "./tokens.stylex";

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
      <div
        {...stylex.props(host.boxless, styles)}
        onPointerLeave={() => setHovered(null)}
      >
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
    pointerEvents: "none",
    inset: 0,
    zIndex: -1,
    borderRadius: radius.row,
    backgroundColor: surface.hover,
  },
  segment: {
    borderRadius: radius.segment,
    backgroundColor: surface.elevated,
    boxShadow: "var(--shadow-surface-card)",
  },
});

export function ItemHighlight({
  layoutId,
  variant = "hover",
  styles,
}: {
  layoutId: string;
  variant?: "hover" | "segment";
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  const reduced = useReducedMotion();
  return (
    <anim.span
      aria-hidden
      data-slot="item-highlight"
      initial={false}
      layoutId={layoutId}
      transition={reduced ? reducedSelectionTransition : selectionTransition}
      {...stylex.props(
        highlight.fill,
        variant === "segment" && highlight.segment,
        styles,
      )}
    />
  );
}
