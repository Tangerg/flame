import { selectionTransition } from "@flame/ui";
import { type Transition } from "motion/react";
import { useMemo } from "react";
import { useAppearanceProjection, type AppearanceProjection } from "@/lib/appearance";
export function createMotion(appearance: AppearanceProjection) {
  function scaled(duration: "fastMs" | "mediumMs" | "slowMs"): Transition {
    const t = {} as Transition;
    Object.defineProperty(t, "duration", {
      enumerable: true,
      get: () => (appearance.read().motion[duration] / 1000) * appearance.read().motionScale,
    });
    Object.defineProperty(t, "ease", {
      enumerable: true,
      get: () => appearance.read().motion.easeOut,
    });
    return t;
  }

  function scaledSpring(duration: "fastMs" | "mediumMs" | "slowMs"): Transition {
    const t = { type: "spring", bounce: 0 } as Transition;
    Object.defineProperty(t, "duration", {
      enumerable: true,
      get: () => (appearance.read().motion[duration] / 1000) * appearance.read().motionScale,
    });
    return t;
  }

  const disclosureTransition: Transition = scaled("mediumMs");

  const disclosureExitTransition: Transition = scaled("fastMs");

  const glyphSwapTransition: Transition = scaledSpring("slowMs");

  const enterUp = {
    initial: { top: 6 },
    animate: { top: 0 },
    transition: disclosureTransition,
  };

  return { disclosureTransition, disclosureExitTransition, glyphSwapTransition, enterUp };
}

export function useMotion() {
  const appearance = useAppearanceProjection();
  return useMemo(() => createMotion(appearance), [appearance]);
}

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
