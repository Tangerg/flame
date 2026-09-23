import * as stylex from "@stylexjs/stylex";
import type { ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  port: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
});

interface Props {
  className?: string;
  children: ReactNode;
  ref?: Ref<HTMLDivElement>;
}

export function ScrollArea({ className, children, ref }: Props) {
  const styled = stylex.props(styles.port);
  return (
    <div ref={ref} {...styled} className={cn(styled.className, className)}>
      {children}
    </div>
  );
}
