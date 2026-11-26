import * as stylex from "@stylexjs/stylex";
import { clsx } from "clsx";
import { type CSSProperties } from "react";
import { useCallback, useLayoutEffect, useRef, useState } from "react";
interface OverflowLabelStyle extends CSSProperties {
  "--agent-overflow-distance": string;
  "--agent-overflow-duration": string;
}

interface Props {
  text: string;
  onFit: (viewport: HTMLSpanElement) => void;
}

const styles = stylex.create({
  label: { minWidth: 0, flex: 1 },
  track: { display: "inline-block", minWidth: "max-content" },
});

const viewportClass = "agent-overflow-label";

function overflowDistance(viewport: Element, track: Element): number {
  return Math.max(0, track.getBoundingClientRect().width - viewport.clientWidth);
}

export function agentLabelOverflows(trigger: Element): boolean {
  const viewport = trigger.querySelector(`.${viewportClass}`);
  const track = viewport?.firstElementChild;
  return !!viewport && !!track && overflowDistance(viewport, track) > 0;
}

export function AgentOverflowLabel({ text, onFit }: Props) {
  const viewportRef = useRef<HTMLSpanElement>(null);
  const trackRef = useRef<HTMLSpanElement>(null);
  const [geometry, setGeometry] = useState({ distance: 0, duration: 0 });

  const measure = useCallback(() => {
    const viewport = viewportRef.current;
    const track = trackRef.current;
    if (!viewport || !track) return;

    const distance = overflowDistance(viewport, track);
    const fontSize = Number.parseFloat(getComputedStyle(track).fontSize) || 13;
    const duration = distance > 0 ? distance / (fontSize * 2) : 0;
    setGeometry((current) =>
      Math.abs(current.distance - distance) < 0.5 && Math.abs(current.duration - duration) < 0.01
        ? current
        : { distance, duration },
    );
    if (distance === 0) onFit(viewport);
  }, [onFit]);

  useLayoutEffect(() => {
    measure();
    if (typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver(measure);
    if (viewportRef.current) observer.observe(viewportRef.current);
    if (trackRef.current) observer.observe(trackRef.current);
    return () => observer.disconnect();
  }, [measure, text]);

  const overflowing = geometry.distance > 0;

  const style: OverflowLabelStyle = {
    "--agent-overflow-distance": `${geometry.distance}px`,
    "--agent-overflow-duration": `${geometry.duration}s`,
  };

  return (
    <span
      ref={viewportRef}
      className={clsx(viewportClass, "truncate-fade", stylex.props(styles.label).className)}
      data-overflowing={overflowing ? "" : undefined}
    >
      <span
        ref={trackRef}
        className={clsx("agent-overflow-track", stylex.props(styles.track).className)}
        data-overflowing={overflowing ? "" : undefined}
        style={style}
      >
        {text}
      </span>
    </span>
  );
}
