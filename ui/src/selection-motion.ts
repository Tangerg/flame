import type { Transition } from "motion/react";

function duration(): number {
  if (typeof document === "undefined") return 0;
  const tokens = getComputedStyle(document.documentElement);
  const milliseconds = Number.parseFloat(
    tokens.getPropertyValue("--dur-slow-base"),
  );
  const scale = Number.parseFloat(tokens.getPropertyValue("--motion-scale"));
  return Number.isFinite(milliseconds) && Number.isFinite(scale)
    ? Math.max(0, (milliseconds * scale) / 1000)
    : 0;
}

export const selectionTransition: Transition = {
  type: "spring",
  bounce: 0,
  get duration() {
    return duration();
  },
};

export const reducedSelectionTransition: Transition = { duration: 0 };
