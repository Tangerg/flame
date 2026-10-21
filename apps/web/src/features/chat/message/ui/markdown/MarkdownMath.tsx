import { type ComponentPropsWithoutRef } from "react";
import { useT } from "@/lib/i18n/index";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
export function MarkdownMath({ children, ...props }: ComponentPropsWithoutRef<"span">) {
  const t = useT();
  const scroll = useScrollReach();
  const reachable = scroll.tabIndex === 0;
  return (
    <span
      {...props}
      {...scroll}
      dir="ltr"
      role={reachable ? "region" : undefined}
      aria-label={reachable ? t("message.math.expression") : undefined}
      data-focus-inset={reachable ? "" : undefined}
    >
      {children}
    </span>
  );
}
