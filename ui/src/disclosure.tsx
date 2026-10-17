import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { ChevronRight } from "@flame/icons/react";
import { CollapsiblePrimitive } from "./primitives/collapsible";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { layout, space, surface, type } from "./tokens.stylex";

const styles = stylex.create({
  root: {
    display: "grid",
    gridTemplateColumns: {
      default: "minmax(0,1fr) auto",
      "@container (max-width: 440px)": "minmax(0,1fr)",
    },
    alignItems: "center",
    columnGap: space.s3,
    minWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":is([data-focused=true])": surface.selected,
    },
  },
  row: {
    paddingInline: layout.cardPadding,
    paddingBlock: space.s3,
    backgroundColor: {
      default: "transparent",
      ":hover": { "@media (hover: hover)": surface.hover },
    },
  },
  trigger: {
    width: "100%",
    height: "auto",
    justifyContent: "flex-start",
    textAlign: "start",
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    paddingBlock: space.s1,
    paddingInline: 0,
  },
  glyph: {
    flexShrink: 0,
    transform: {
      default: "rotate(0deg)",
      ":dir(rtl)": "rotate(180deg)",
      ":is([data-panel-open] > *)": "rotate(90deg)",
      ":is([data-panel-open] > *):dir(rtl)": "rotate(90deg)",
    },
  },
  title: { minWidth: 0, flex: 1, overflowWrap: "anywhere" },
  content: {
    gridColumn: "1 / -1",
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.controlGap,
    paddingBlock: space.s3,
  },
  rowContent: {
    paddingBlockStart: space.s4,
    marginBlockStart: space.s2,
    borderBlockStartWidth: "var(--control-edge-width)",
    borderBlockStartStyle: "solid",
    borderBlockStartColor: surface.divider,
  },
  actions: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: layout.controlGap,
    justifyContent: "flex-end",
  },
});
export function Disclosure({
  title,
  children,
  open,
  defaultOpen,
  onOpenChange,
  variant = "plain",
  actions,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title" | "onChange"> & {
  title: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  variant?: "plain" | "row";
  actions?: ReactNode;
}) {
  return (
    <CollapsiblePrimitive.Root
      open={open}
      defaultOpen={defaultOpen}
      onOpenChange={onOpenChange}
      render={
        <section
          {...props}
          {...stylex.props(styles.root, variant === "row" && styles.row)}
        />
      }
    >
      <CollapsiblePrimitive.Trigger
        render={
          <Button
            variant={variant === "row" ? "bare" : "ghost"}
            size="sm"
            styles={styles.trigger}
          />
        }
      >
        {variant === "plain" && (
          <Icon
            glyph={ChevronRight}
            size="xs"
            {...stylex.props(styles.glyph)}
          />
        )}
        <span {...stylex.props(styles.title, type.uiSm)}>{title}</span>
        {variant === "row" && (
          <Icon
            glyph={ChevronRight}
            size="xs"
            {...stylex.props(styles.glyph)}
          />
        )}
      </CollapsiblePrimitive.Trigger>
      {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
      <CollapsiblePrimitive.Panel
        {...stylex.props(
          styles.content,
          variant === "row" && styles.rowContent,
          type.uiSm,
        )}
      >
        {children}
      </CollapsiblePrimitive.Panel>
    </CollapsiblePrimitive.Root>
  );
}
