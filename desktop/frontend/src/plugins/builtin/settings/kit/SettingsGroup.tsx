import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { Surface } from "@/ui";

const styles = stylex.create({
  group: { overflow: "hidden", width: "100%", containerType: "inline-size" },
});

export function SettingsGroup({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <Surface
      {...props}
      variant="group"
      data-focus-inset=""
      inset="none"
      className={cn(stylex.props(styles.group).className, className)}
    >
      {children}
    </Surface>
  );
}
