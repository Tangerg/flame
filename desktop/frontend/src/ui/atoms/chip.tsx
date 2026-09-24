import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color, corner, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { reveal } from "./reveal";
import { Icon, type IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { ButtonPrimitive } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

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
  open: {
    display: "inline-flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    color: { default: "inherit", ":hover": color.fg },
    cursor: "default",
  },
  close: {
    scale: "calc(0.96 + 0.04 * var(--reveal, 1))",
    transitionProperty: "opacity, scale, background-color, color",
    transitionDuration: "var(--dur-fast)",
    transitionTimingFunction: motion.easeState,
  },
});

interface Props {
  icon?: IconName;
  children: ReactNode;
  title?: string;
  onClose?: () => void;
  onOpen?: () => void;
  kind?: "reference" | "attached";
  closeLabel?: string;
}

export function Chip({
  icon,
  children,
  title,
  onClose,
  onOpen,
  kind = "reference",
  closeLabel,
}: Props) {
  const t = useT();
  const face = (
    <>
      {icon && <Icon name={icon} size="xs" />}
      <span {...stylex.props(chipStyles.value)}>{children}</span>
    </>
  );
  return (
    <Tooltip label={title}>
      <span
        data-slot="chip"
        data-kind={kind}
        {...stylex.props(reveal.host, chipStyles.chip, chipStyles[kind], corner.pill, type.uiSm)}
      >
        {onOpen ? (
          <ButtonPrimitive
            type="button"
            aria-label={title}
            onClick={onOpen}
            {...stylex.props(chipStyles.open)}
          >
            {face}
          </ButtonPrimitive>
        ) : (
          face
        )}
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
