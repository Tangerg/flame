import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { Surface } from "@/ui";

// The group clips its rows because the first and last of them meet its corner.
const styles = stylex.create({ group: { overflow: "hidden" } });

export function SettingsGroup({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <Surface
      {...props}
      variant="group"
      inset="none"
      className={cn(stylex.props(styles.group).className, className)}
    >
      {children}
    </Surface>
  );
}
