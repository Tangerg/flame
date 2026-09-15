// Every preset's duration multiplies by the published motion scale AT READ TIME, so the
// user's preference reaches every animation without a hook at each call site.

import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

// `duration` is a live getter; framer-motion samples it once per animation start.
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

/** A surface opening, closing, or arriving. One preset, because the rung it used to have
 *  of its own sat 20ms from this one — a fifth of a frame, which is a rung the ladder
 *  cannot express and a reader cannot see. */
export const disclosureTransition: Transition = scaled("mediumMs");

/** Leaving, which is a rung faster than arriving.
 *
 *  Arriving is information: the reader has to notice the thing and read where it came from.
 *  Leaving is not — it is the interface getting out of the way, and matching the entrance
 *  makes the reader wait on an animation that has nothing left to say. The rung below is the
 *  whole change; there is no new number here, because a duration the ladder cannot express is
 *  one nobody can hold. */
export const disclosureExitTransition: Transition = scaled("fastMs");

/** A selection travelling BETWEEN elements — the one motion CSS cannot express, since a
 *  transition animates a property within one element. */
export const selectionTransition: Transition = scaled("fastMs");

/** A glyph REPLACING another in the same box. A spring rather than a tween because the two
 *  travel through one 16px square and an eased cross-fade reads as a dissolve; `bounce: 0`
 *  so a control this small settles without overshoot.
 *
 *  Here rather than at the call site because a literal duration there does not scale: the
 *  theme toggle carried `{ type: "spring", duration: 0.3, bounce: 0 }`, and with the motion
 *  preference at zero it went on animating — 25 style frames — while every other animation
 *  in the app stopped. 0.3s was never arbitrary; it is this ladder's `slowMs`. */
function scaledSpring(duration: "fastMs" | "mediumMs" | "slowMs"): Transition {
  const t = { type: "spring", bounce: 0 } as Transition;
  Object.defineProperty(t, "duration", {
    enumerable: true,
    get: () => (visualStyleMotion()[duration] / 1000) * motionScale(),
  });
  return t;
}

export const glyphSwapTransition: Transition = scaledSpring("slowMs");

/** PRESENCE only. Adding `layout` costs a measurement on every render of the holder — and
 *  the composer re-renders on every keystroke. */
export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

/** A step arriving in a chain that is still being written.
 *
 *  Its own preset because a step is not a turn: a turn is a whole exchange entering the
 *  transcript and travels 6px on the medium rung, a step is one line joining a chain the
 *  reader is already watching and would read as a lurch at that distance. Presence only —
 *  steps do not leave a turn, and `layout` would measure the holder on every streamed token. */
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
