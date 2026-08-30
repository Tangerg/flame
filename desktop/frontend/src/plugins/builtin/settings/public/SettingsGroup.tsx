import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { Surface } from "@/ui";

/**
 * The group owns the one outer edge; child `SettingRow`s own only the separators between
 * siblings, so every pane shares a form grammar without copying border/fill decisions.
 */
export function SettingsGroup({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <Surface
      {...props}
      inset="none"
      className={cn(
        "overflow-hidden border-[length:var(--control-edge-width)] border-field bg-transparent",
        className,
      )}
    >
      {children}
    </Surface>
  );
}
