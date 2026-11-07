import { type ComponentPropsWithoutRef, type KeyboardEvent } from "react";
import { scrollWithHorizontalArrow } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
export function MarkdownMath({ children, onKeyDown, ...props }: ComponentPropsWithoutRef<"span">) {
  const t = useT();
  const scroll = useScrollReach();
  const reachable = scroll.tabIndex === 0;
  const interaction = reachable
    ? {
        role: "region" as const,
        onKeyDown(event: KeyboardEvent<HTMLSpanElement>) {
          onKeyDown?.(event);
          scrollWithHorizontalArrow(event);
        },
      }
    : { role: undefined, onKeyDown };
  return (
    <span
      {...props}
      {...scroll}
      {...interaction}
      dir="ltr"
      aria-label={reachable ? t("message.math.expression") : undefined}
      data-focus-inset={reachable ? "" : undefined}
    >
      {children}
    </span>
  );
}
