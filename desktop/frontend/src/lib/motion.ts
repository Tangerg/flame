// Every preset's duration multiplies by the published motion scale AT READ TIME, so the
// user's preference reaches every animation without a hook at each call site.

import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

// `duration` is a live getter; framer-motion samples it once per animation start.
function scaled(duration: "fastMs" | "mediumMs"): Transition {
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

/** A surface opening, closing, or arriving. One preset, because the rung it used to have
 *  of its own sat 20ms from this one — a fifth of a frame, which is a rung the ladder
 *  cannot express and a reader cannot see. */
export const disclosureTransition: Transition = scaled("mediumMs");

/** A selection travelling BETWEEN elements — the one motion CSS cannot express, since a
 *  transition animates a property within one element. */
export const selectionTransition: Transition = scaled("fastMs");

/** PRESENCE only. Adding `layout` costs a measurement on every render of the holder — and
 *  the composer re-renders on every keystroke. */
export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: disclosureTransition,
};
