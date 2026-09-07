import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color } from "@/styles/tokens.stylex";
import { Button, type ButtonProps } from "@/ui/atoms";
import { Icon } from "@/ui/icons";

interface Props extends Omit<ButtonProps, "children" | "size"> {
  leading: ReactNode;
  label: string;
  /** `gives` yields its label first when the row is short; `holds` keeps it as long as it can.
   *  Shrinking every chip equally truncates all of them to initials. */
  shrink?: "holds" | "gives";
}

/** The middle grid track is the only one that may shrink, so a chip bottoms out at its glyph
 *  and chevron instead of a sliver whose contents spill onto the next control. `title` names
 *  the current value because that is where the label survives. */
// The middle track is the only one that may shrink, and how EAGERLY it does is the chip's own
// decision — a row of chips that all yield equally truncates every one of them to initials.
const chipStyles = stylex.create({
  grid: { display: "grid", gridTemplateColumns: "auto minmax(0, auto) auto" },
  leading: { display: "flex", alignItems: "center" },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  chevron: { color: color.fgFaint },
  holds: { flexShrink: 1 },
  gives: { flexShrink: 12 },
});

export function AgentComposerChip({
  variant = "ghost",
  leading,
  label,
  className,
  shrink = "holds",
  title,
  ...props
}: Props) {
  return (
    <Button
      variant={variant}
      size="md"
      chip
      press={false}
      title={title ?? label}
      styles={[chipStyles.grid, shrink === "gives" ? chipStyles.gives : chipStyles.holds]}
      className={className}
      {...props}
    >
      <span {...stylex.props(chipStyles.leading)}>{leading}</span>
      <span data-slot="composer-chip-label" {...stylex.props(chipStyles.label)}>
        {label}
      </span>
      <Icon name="chevron-down" size="sm" {...stylex.props(chipStyles.chevron)} />
    </Button>
  );
}
