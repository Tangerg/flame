import { type ComponentPropsWithoutRef } from "react";
import { useT } from "@/lib/i18n/index";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
export function MarkdownMath({ children, ...props }: ComponentPropsWithoutRef<"span">) {
  const t = useT();
  const scroll = useScrollReach();
  return (
    <span
      {...props}
      {...scroll}
      dir="ltr"
      role="region"
      aria-label={t("message.math.expression")}
      data-focus-inset=""
    >
      {children}
    </span>
  );
}
