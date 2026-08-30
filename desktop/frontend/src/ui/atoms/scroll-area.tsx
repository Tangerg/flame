import type { CSSProperties, ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

interface Props {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
  /** For dense surfaces where WebKit's overlay thumb crowds row content. */
  hideScrollbar?: boolean;
}

// Native scrollbar rather than a headless scroll-area primitive: virtual track overhead
// buys nothing on the surfaces this is used on.
export function ScrollArea({ className, style, children, hideScrollbar, ref }: Props) {
  // `hideScrollbar` DROPS `.panel-scroll` rather than layering on it: that class's
  // `::-webkit-scrollbar { width: 10px }` lives in globals.css, which comes after Tailwind
  // utilities in the cascade, and at equal specificity source order wins.
  return (
    <div
      ref={ref}
      className={cn(
        hideScrollbar
          ? "flex-1 min-h-0 overflow-y-auto overflow-x-hidden overscroll-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          : "panel-scroll",
        className,
      )}
      style={style}
    >
      {children}
    </div>
  );
}
