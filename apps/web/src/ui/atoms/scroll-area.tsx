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
  // Content clips at the padding edge, before the inset focus perimeter.
  focusable: {
    borderWidth: "var(--focus-inset-edge)",
    borderStyle: "solid",
    borderColor: "transparent",
  },
});

interface Props {
  className?: string;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
  tabIndex?: number;
  role?: string;
  "aria-label"?: string;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  onCopy?: ClipboardEventHandler<HTMLDivElement>;
}

export function ScrollArea({ className, children, ref, ...attributes }: Props) {
  const styled = stylex.props(styles.port, attributes.tabIndex !== undefined && styles.focusable);
  return (
    <div ref={ref} {...attributes} {...styled} className={cn(styled.className, className)}>
      {children}
    </div>
  );
}
