import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

// `duration` and `ease` are live getters, not values: motion samples them when an animation
// starts, which is how the user's motion preference reaches every preset without a hook at
// each call site. A plain literal here would freeze at module load.
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

/** A surface opening, closing, or arriving. */
export const disclosureTransition: Transition = scaled("mediumMs");

/** Leaving, a rung faster than arriving. */
export const disclosureExitTransition: Transition = scaled("fastMs");

/** A selection travelling BETWEEN elements — the one motion CSS cannot express, since a
 *  transition animates a property within a single element. */
export const selectionTransition: Transition = scaled("fastMs");

/** A glyph replacing another in the same box. */
export const glyphSwapTransition: Transition = scaledSpring("slowMs");

// Presence only, never `layout`: it measures the holder on every render, and these sit in the
// composer and the transcript, which re-render per keystroke and per streamed token.
export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

/** A step joining a chain the reader is already watching. */
export const stepEnter = {
  initial: { opacity: 0, y: 4 },
  animate: { opacity: 1, y: 0 },
  transition: selectionTransition,
};

/** A whole exchange entering the transcript. */
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4, transition: disclosureExitTransition },
  transition: disclosureTransition,
};
