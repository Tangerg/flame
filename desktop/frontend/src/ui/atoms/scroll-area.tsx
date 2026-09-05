import type { ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

interface Props {
  className?: string;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
  hideScrollbar?: boolean;
}

export function ScrollArea({ className, children, hideScrollbar, ref }: Props) {
  return (
    <div
      ref={ref}
      className={cn(
        "min-h-0 flex-1 overflow-y-auto overscroll-contain",
        hideScrollbar
          ? "overflow-x-hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          : "panel-scroll",
        className,
      )}
    >
      {children}
    </div>
  );
}
