import * as stylex from "@stylexjs/stylex";
import {
  type ClipboardEventHandler,
  type KeyboardEventHandler,
  type ReactNode,
  type Ref,
} from "react";
import { cn } from "@/lib/classNames";
const styles = stylex.create({
  port: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
});

interface Props {
  className?: string;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
  // Set these to make a region whose content has no focusable elements
  // keyboard-scrollable (WCAG 2.1.1) — e.g. a plain-text file viewer.
  tabIndex?: number;
  role?: string;
  "aria-label"?: string;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  onCopy?: ClipboardEventHandler<HTMLDivElement>;
}

export function ScrollArea({ className, children, ref, ...attributes }: Props) {
  const styled = stylex.props(styles.port);
  return (
    <div ref={ref} {...attributes} {...styled} className={cn(styled.className, className)}>
      {children}
    </div>
  );
}
