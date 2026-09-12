import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color, corner, space, surface, type, weight } from "@/styles/tokens.stylex";
import { reveal } from "./reveal";
import { Icon, type IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { ButtonPrimitive } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

// The close button also grows into place. `--reveal` is 0 or 1, so one channel drives both the
// fade and the scale without a second custom property.
const chipStyles = stylex.create({
  chip: {
    display: "inline-flex",
    height: "var(--control-height-sm)",
    alignItems: "center",
    gap: space.s1_5,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    paddingLeft: space.s2_5,
    paddingRight: space.s1,
    fontWeight: weight.regular,
  },
  reference: { backgroundColor: surface.accentBadge, color: color.fgSoft },
  attached: { backgroundColor: surface.surface2, color: color.fgMuted },
  // The value is machine text and it is capped: a chip that grows with its content pushes the
  // rest of the row off the end instead of yielding.
  value: {
    maxWidth: "220px",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontFamily: "var(--font-mono)",
  },
  closeBox: {
    display: "grid",
    height: space.s5,
    width: space.s5,
    placeItems: "center",
    borderWidth: 0,
    backgroundColor: { default: "transparent", ":hover": surface.hover },
    color: { default: color.fgFaint, ":hover": color.fg },
  },
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
  /** What the chip is for. `reference` is something the reader named; `attached` came along. */
  kind?: "reference" | "attached";
  /** Names what is being removed, for a row where "Remove" alone does not say which one. */
  closeLabel?: string;
}

export function Chip({ icon, children, title, onClose, kind = "reference", closeLabel }: Props) {
  const t = useT();
  return (
    <Tooltip label={title}>
      <span
        // What it is and which kind, as attributes: they are what a test can hold onto, and a
        // generated class name is not a contract.
        data-slot="chip"
        data-kind={kind}
        {...stylex.props(reveal.host, chipStyles.chip, chipStyles[kind], corner.pill, type.uiSm)}
      >
        {icon && <Icon name={icon} size="xs" />}
        <span {...stylex.props(chipStyles.value)}>{children}</span>
        {onClose && (
          <ButtonPrimitive
            data-reveal="hover"
            type="button"
            {...stylex.props(reveal.shown, chipStyles.closeBox, corner.pill, chipStyles.close)}
            onClick={onClose}
            aria-label={closeLabel ?? t("common.remove")}
          >
            <Icon name="x" size="xs" />
          </ButtonPrimitive>
        )}
      </span>
    </Tooltip>
  );
}
