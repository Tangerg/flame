import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { Surface } from "@/ui";

export function SettingsGroup({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <Surface {...props} variant="group" inset="none" className={cn("overflow-hidden", className)}>
      {children}
    </Surface>
  );
}
