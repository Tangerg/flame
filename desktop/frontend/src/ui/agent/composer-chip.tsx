import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color } from "@/styles/tokens.stylex";
import { Button, type ButtonProps } from "@/ui/atoms";
import { Icon } from "@/ui/icons";

interface Props extends Omit<ButtonProps, "children" | "size"> {
  leading: ReactNode;
  label: string;
  shrink?: "holds" | "gives";
  labelVisibility?: "always" | "wide";
}

const chipStyles = stylex.create({
  grid: { display: "grid", gridTemplateColumns: "auto minmax(0, auto) auto" },
  leading: { display: "flex", alignItems: "center" },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  chevron: { color: color.fgFaint },
  holds: { flexShrink: 0 },
  gives: { flexShrink: 1, minWidth: 0 },
  wide: { display: { default: "none", "@container composer (min-width: 480px)": "block" } },
});

export function AgentComposerChip({
  variant = "ghost",
  leading,
  label,
  className,
  shrink = "holds",
  title,
  labelVisibility = "always",
  ...props
}: Props) {
  return (
    <Button
      variant={variant}
      size="md"
      chip
      title={title ?? label}
      styles={[chipStyles.grid, shrink === "gives" ? chipStyles.gives : chipStyles.holds]}
      className={className}
      {...props}
    >
      <span {...stylex.props(chipStyles.leading)}>{leading}</span>
      <span
        data-slot="composer-chip-label"
        {...stylex.props(chipStyles.label, labelVisibility === "wide" && chipStyles.wide)}
      >
        {label}
      </span>
      <Icon name="chevron-down" size="sm" {...stylex.props(chipStyles.chevron)} />
    </Button>
  );
}
