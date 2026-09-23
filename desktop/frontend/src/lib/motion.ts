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

export const stepEnter = {
  initial: { opacity: 0, y: 4 },
  animate: { opacity: 1, y: 0 },
  transition: selectionTransition,
};

export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4, transition: disclosureExitTransition },
  transition: disclosureTransition,
};
