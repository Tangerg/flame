import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { reveal } from "./reveal";
import { Icon, type IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { ButtonPrimitive } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

// The close button also grows into place. `--reveal` is 0 or 1, so one channel drives both the
// fade and the scale without a second custom property.
const chipStyles = stylex.create({
  close: {
    scale: "calc(0.96 + 0.04 * var(--reveal, 1))",
    // One transition list, stated once: the fade, the growth and the hover recolour. Split
    // across two declarations the later one wins and the others simply stop animating.
    transitionProperty: "opacity, scale, background-color, color",
    transitionDuration: "var(--dur-fast)",
  },
});

interface Props {
  icon?: IconName;
  children: ReactNode;
  title?: string;
  onClose?: () => void;
}

export function Chip({ icon, children, title, onClose }: Props) {
  const t = useT();
  return (
    <Tooltip label={title}>
      <span
        className={cn(
          stylex.props(reveal.host).className,
          "inline-flex h-[var(--control-height-sm)] items-center gap-1.5 rounded-pill border-[length:var(--control-edge-width)] border-field bg-accent-badge pl-2.5 pr-1 text-ui-sm font-normal text-fg-soft",
        )}
      >
        {icon && <Icon name={icon} size="xs" />}
        <span className="max-w-[220px] truncate font-mono">{children}</span>
        {onClose && (
          <ButtonPrimitive
            data-reveal="hover"
            type="button"
            className={cn(
              stylex.props(reveal.shown, chipStyles.close).className,
              "grid h-5 w-5 place-items-center rounded-pill border-0 bg-transparent text-fg-faint hover:bg-hover hover:text-fg",
            )}
            onClick={onClose}
            aria-label={t("common.remove")}
          >
            <Icon name="x" size="xs" />
          </ButtonPrimitive>
        )}
      </span>
    </Tooltip>
  );
}
