import { useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  type ElementView,
  type Frame,
  type SlideLayout,
  type SlideView,
} from "../contracts/content";
import { EMU_PER_INCH, label } from "./slides-presentation";

const NUDGE = EMU_PER_INCH / 10;

export function Canvas({
  layout,
  slide,
  selected,
  editable,
  onSelect,
  onFrame,
  onDelete,
  t,
}: {
  layout: SlideLayout;
  slide: SlideView;
  selected?: string;
  editable: boolean;
  onSelect(id: string | undefined): void;
  onFrame(element: ElementView, frame: Frame): void;
  onDelete(element: ElementView): void;
  t: Translate;
}) {
  const surface = useRef<HTMLDivElement>(null);
  const [gesture, setGesture] = useState<{
    id: string;
    mode: "move" | "resize";
    pointer: number;
    x: number;
    y: number;
    start: Frame;
    frame: Frame;
  }>();
  const begin = (
    event: PointerEvent<HTMLElement>,
    element: ElementView,
    mode: "move" | "resize",
  ) => {
    event.stopPropagation();
    onSelect(element.id);
    if (!editable || !element.frame || event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    setGesture({
      id: element.id,
      mode,
      pointer: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      start: element.frame,
      frame: element.frame,
    });
  };
  const move = (event: PointerEvent<HTMLElement>) => {
    if (!gesture || event.pointerId !== gesture.pointer || !surface.current)
      return;
    const scale = layout.width / surface.current.getBoundingClientRect().width;
    const dx = Math.round((event.clientX - gesture.x) * scale);
    const dy = Math.round((event.clientY - gesture.y) * scale);
    const { start } = gesture;
    setGesture({
      ...gesture,
      frame:
        gesture.mode === "move"
          ? { ...start, x: start.x + dx, y: start.y + dy }
          : {
              ...start,
              width: Math.max(0, start.width + dx),
              height: Math.max(0, start.height + dy),
            },
    });
  };
  const end = (event: PointerEvent<HTMLElement>, element: ElementView) => {
    if (!gesture || event.pointerId !== gesture.pointer) return;
    setGesture(undefined);
    if (
      gesture.frame.x !== gesture.start.x ||
      gesture.frame.y !== gesture.start.y ||
      gesture.frame.width !== gesture.start.width ||
      gesture.frame.height !== gesture.start.height
    )
      onFrame(element, gesture.frame);
  };
  const keys = (event: KeyboardEvent<HTMLElement>, element: ElementView) => {
    if (event.key === "Escape") {
      onSelect(undefined);
      return;
    }
    if (!editable) return;
    if (event.key === "Delete" || event.key === "Backspace") {
      event.preventDefault();
      onDelete(element);
      return;
    }
    const step = event.shiftKey ? EMU_PER_INCH : NUDGE;
    const delta = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    }[event.key];
    if (!delta || !element.frame) return;
    event.preventDefault();
    onFrame(element, {
      ...element.frame,
      x: element.frame.x + delta[0]!,
      y: element.frame.y + delta[1]!,
    });
  };
  const percent = (value: number, total: number) => `${(value / total) * 100}%`;
  return (
    <div
      ref={surface}
      role="group"
      aria-label={t("slides.canvas", { number: slide.number })}
      aria-description={t("slides.keyboard")}
      onPointerDown={() => onSelect(undefined)}
      style={{
        position: "relative",
        width: "100%",
        aspectRatio: `${layout.width} / ${layout.height}`,
        overflow: "hidden",
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--shape-sm)",
        touchAction: "none",
      }}
    >
      {slide.elements.map((element) => {
        const frame =
          gesture?.id === element.id ? gesture.frame : element.frame;
        if (!frame) return null;
        const active = element.id === selected;
        return (
          <button
            key={element.id}
            type="button"
            aria-pressed={active}
            aria-label={`${t(`slides.kind.${element.kind}`)} · ${label(element, t)}`}
            onPointerDown={(event) => begin(event, element, "move")}
            onPointerMove={move}
            onPointerUp={(event) => end(event, element)}
            onPointerCancel={() => setGesture(undefined)}
            onKeyDown={(event) => keys(event, element)}
            onFocus={() => onSelect(element.id)}
            style={{
              position: "absolute",
              left: percent(frame.x, layout.width),
              top: percent(frame.y, layout.height),
              width: percent(frame.width, layout.width),
              height: percent(frame.height, layout.height),
              minWidth: 6,
              minHeight: 6,
              transform: element.rotation
                ? `rotate(${element.rotation}deg)`
                : undefined,
              padding: 2,
              margin: 0,
              overflow: "hidden",
              textAlign: "start",
              font: "inherit",
              fontSize: "0.7rem",
              lineHeight: 1.25,
              color: "var(--color-text-soft)",
              cursor: editable && element.frame ? "move" : "pointer",
              background:
                element.kind === "chart" || element.kind === "table"
                  ? "var(--color-info-wash)"
                  : element.kind === "picture"
                    ? "var(--color-sunken)"
                    : element.kind === "text"
                      ? "transparent"
                      : "var(--color-surface-2)",
              border: `1px ${active ? "solid" : "dashed"} ${
                active ? "var(--color-accent)" : "var(--color-border-strong)"
              }`,
              outline: active ? "2px solid var(--color-accent)" : "none",
              outlineOffset: 1,
              zIndex: active ? 1 : undefined,
            }}
          >
            {label(element, t)}
            {active && editable && element.frame && (
              <span
                aria-hidden
                onPointerDown={(event) => begin(event, element, "resize")}
                onPointerMove={(event) => {
                  event.stopPropagation();
                  move(event);
                }}
                onPointerUp={(event) => {
                  event.stopPropagation();
                  end(event, element);
                }}
                style={{
                  position: "absolute",
                  right: 0,
                  bottom: 0,
                  width: 10,
                  height: 10,
                  cursor: "nwse-resize",
                  background: "var(--color-accent)",
                }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}
