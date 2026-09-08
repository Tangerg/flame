import * as stylex from "@stylexjs/stylex";
import { motion, space } from "@/styles/tokens.stylex";
import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  surface: {
    overflow: "hidden",
    borderRadius: "var(--radius-composer)",
    transitionProperty: "box-shadow",
    transitionDuration: motion.med,
    transitionTimingFunction: "var(--ease-out)",
  },
  footer: {
    display: "flex",
    flexWrap: "nowrap",
    alignItems: "center",
    gap: space.s1_5,
    paddingRight: "var(--density-composer-footer-end)",
    paddingBottom: "var(--density-composer-footer)",
    paddingLeft: "var(--density-composer-footer)",
  },
});

export function AgentComposerSurface({
  className,
  children,
  ref,
  ...props
}: ComponentPropsWithoutRef<"div"> & { ref?: Ref<HTMLDivElement> }) {
  return (
    <div
      {...props}
      ref={ref}
      // `agent-composer-glass` is the composer's material — a backdrop filter and the edge
      // the visual style owns — and stays in `globals.css` with the rest of the window's chrome.
      className={cn("agent-composer-glass", stylex.props(styles.surface).className, className)}
    >
      {children}
    </div>
  );
}

/** The chip row under the input. Owns the density padding and the control-size overrides the
 *  chips read, so a chip stays the composer's size wherever it is contributed from. `labelled`
 *  is the fitted state; its `data-measuring` companion is toggled on the ref for the length of
 *  the measuring reflow, which is why that one is not a prop. */
export function AgentComposerFooter({
  labelled,
  ref,
  children,
}: {
  labelled: boolean;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}) {
  return (
    <div
      ref={ref}
      data-slot="composer-footer"
      data-labelled={labelled ? "" : undefined}
      // `agent-composer-footer` is the measuring key: `globals.css` holds the chips still for
      // one reflow through it, so it is a mechanism binding rather than a style.
      className={cn("agent-composer-footer", stylex.props(styles.footer).className)}
    >
      {children}
    </div>
  );
}
