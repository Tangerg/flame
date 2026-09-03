import { useCallback, useLayoutEffect, useRef, useState } from "react";

/**
 * Whether the composer's toolbar has room for its chips' labels.
 *
 * It is a measurement because CSS cannot answer it. Flexbox shrinks every chip at once, and
 * a chip's label box is sized to its text, so the row arrives at three two-character stubs
 * (`Balanc…`, `GP…`, `Mediu…`) rather than at one chip that gave up its label. Nothing in
 * `flex-shrink`, `minmax()`, negative margins or a container query reaches "hide the label
 * instead of ellipsing it"; each was tried against a screenshot and each still ellipsed.
 *
 * Measuring while collapsed would read the collapsed width and oscillate, so the natural
 * width is read with `data-measuring` on — one attribute the stylesheet uses to un-hide the
 * labels for the length of a single synchronous reflow.
 */
export function useToolbarLabels(): {
  ref: (node: HTMLElement | null) => void;
  labelled: boolean;
} {
  const node = useRef<HTMLElement | null>(null);
  const [labelled, setLabelled] = useState(true);

  const measure = useCallback(() => {
    const element = node.current;
    if (!element) return;
    element.dataset.measuring = "";
    const natural = element.scrollWidth;
    const available = element.clientWidth;
    delete element.dataset.measuring;
    setLabelled(natural <= available);
  }, []);

  useLayoutEffect(() => {
    const element = node.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [measure]);

  const ref = useCallback(
    (next: HTMLElement | null) => {
      node.current = next;
      if (next) measure();
    },
    [measure],
  );

  return { ref, labelled };
}
