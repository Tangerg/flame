// Motion presets — shared easing curves and durations so transitions across
// the app feel like one design system, not a grab bag of values.
//
// The duration on every preset multiplies by the published motion scale at read
// time, so the user's Settings → Motion preference (Off / Fast / Default / Slow)
// ripples through every motion/react animation without each call site touching
// it. Framer-motion reads `transition.duration` on each animate, so a per-access
// getter is fine — no need for hook plumbing at every consumer.

import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

// Build a Transition whose `duration` field is a live getter — reads the
// current scale on every access. Framer-motion samples it once per animation
// start, so the cost is negligible and the user sees the new scale immediately
// after toggling.
function scaled(duration: "fastMs" | "mediumMs" | "disclosureMs"): Transition {
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

export const disclosureTransition: Transition = scaled("disclosureMs");

const contentEnterTransition: Transition = scaled("mediumMs");

/**
 * A selection travelling between elements — the one kind of motion CSS cannot express, since
 * a transition animates a property WITHIN one element. `layoutId` does for free what would
 * otherwise be a hand-measured absolutely-positioned indicator.
 */
export const selectionTransition: Transition = scaled("fastMs");

/**
 * PRESENCE only. Adding `layout` would slide the survivors into the gap instead of letting
 * them jump, at the price of a measurement on every render of whatever holds them — and the
 * composer re-renders on every keystroke.
 */
export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

// Soft enter from a few px below — for new chat messages.
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: contentEnterTransition,
};
