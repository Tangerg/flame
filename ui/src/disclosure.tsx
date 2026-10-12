import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { ChevronRight } from "@flame/icons/react";
import { CollapsiblePrimitive } from "./primitives/collapsible";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { space, surface, type } from "./tokens.stylex";

const styles = stylex.create({
  root: {
    minWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":is([data-focused=true])": surface.selected,
    },
  },
  trigger: {
    width: "100%",
    height: "auto",
    minHeight: "24px",
    justifyContent: "flex-start",
    textAlign: "start",
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    paddingBlock: space.s2,
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
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: space.s2,
    paddingBlock: space.s2,
  },
});
export function Disclosure({
  title,
  children,
  open,
  defaultOpen,
  onOpenChange,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title" | "onChange"> & {
  title: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  return (
    <CollapsiblePrimitive.Root
      open={open}
      defaultOpen={defaultOpen}
      onOpenChange={onOpenChange}
      render={<section {...props} {...stylex.props(styles.root)} />}
    >
      <CollapsiblePrimitive.Trigger
        render={<Button variant="ghost" size="sm" styles={styles.trigger} />}
      >
        <Icon glyph={ChevronRight} size="xs" {...stylex.props(styles.glyph)} />
        <span {...stylex.props(styles.title)}>{title}</span>
      </CollapsiblePrimitive.Trigger>
      <CollapsiblePrimitive.Panel {...stylex.props(styles.content, type.uiSm)}>
        {children}
      </CollapsiblePrimitive.Panel>
    </CollapsiblePrimitive.Root>
  );
}
