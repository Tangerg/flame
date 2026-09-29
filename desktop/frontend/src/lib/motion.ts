import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

function scaled(duration: "fastMs" | "mediumMs" | "slowMs"): Transition {
  const t = {} as Transition;
  Object.defineProperty(t, "duration", {
    enumerable: true,
    get: () => (visualStyleMotion()[duration] / 1000) * motionScale(),
  });
  Object.defineProperty(t, "ease", {
    enumerable: true,
    get: () => visualStyleMotion().easeOut,
  });
  return t;
}

function scaledSpring(duration: "fastMs" | "mediumMs" | "slowMs"): Transition {
  const t = { type: "spring", bounce: 0 } as Transition;
  Object.defineProperty(t, "duration", {
    enumerable: true,
    get: () => (visualStyleMotion()[duration] / 1000) * motionScale(),
  });
  return t;
}

export const disclosureTransition: Transition = scaled("mediumMs");

export const disclosureExitTransition: Transition = scaled("fastMs");

export const selectionTransition: Transition = scaled("fastMs");

export const glyphSwapTransition: Transition = scaledSpring("slowMs");

export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

// Transcript rows sit inside a masked scroller. Animate their painted position
// without fading or transforming the whole surface into a composited layer.
export const stepEnter = {
  initial: { top: 4 },
  animate: { top: 0 },
  transition: selectionTransition,
};

export const enterUp = {
  initial: { top: 6 },
  animate: { top: 0 },
  transition: disclosureTransition,
};
