import type { CSSProperties, ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

interface Props {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
  hideScrollbar?: boolean;
}

export function ScrollArea({ className, style, children, hideScrollbar, ref }: Props) {
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
