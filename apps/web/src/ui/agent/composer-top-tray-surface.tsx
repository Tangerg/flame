import * as stylex from "@stylexjs/stylex";
import { radius } from "@flame/ui/tokens.stylex";
import { type ComponentPropsWithoutRef } from "react";
import { clsx } from "clsx";
const styles = stylex.create({
  surface: {
    position: "relative",
    minWidth: 0,
    overflow: "clip",
    borderTopLeftRadius: radius.composer,
    borderTopRightRadius: radius.composer,
  },
});

export function AgentComposerTopTraySurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  const styled = stylex.props(styles.surface);
  return (
    <div
      {...props}
      data-slot="composer-top-tray-surface"
      className={clsx(styled.className, className)}
    >
      {children}
    </div>
  );
}
