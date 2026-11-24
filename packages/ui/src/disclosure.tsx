import * as stylex from "@stylexjs/stylex";
import { type ComponentPropsWithRef, type ReactNode } from "react";
import { ChevronRight } from "@flame/icons/react";
import { Collapsible as CollapsiblePrimitive } from "@base-ui/react/collapsible";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { layout, space, type, rowInset } from "./tokens.stylex";

const styles = stylex.create({
  root: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    minWidth: 0,
  },
  header: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    columnGap: layout.contentGap,
    rowGap: layout.controlGap,
    minWidth: 0,
  },
  row: {
    paddingInline: rowInset.inline,
    paddingBlock: space.s3,
  },
  // The summary's fill spans the row's inset, matching other list row fills.
  rowTrigger: {
    marginInline: `calc(-1 * ${rowInset.inline})`,
    paddingInline: rowInset.inline,
    maxWidth: "none",
    borderWidth: 0,
    lineHeight: "inherit",
  },
  trigger: {
    width: "auto",
    flexGrow: 1,
    flexShrink: 1,
    flexBasis: "auto",
    minWidth: 0,
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
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.controlGap,
    paddingBlock: space.s3,
  },
  rowContent: {
    paddingBlockStart: space.s4,
    marginBlockStart: space.s2,
  },
  actions: {
    display: "flex",
    flex: "0 1 auto",
    minWidth: 0,
    maxWidth: "100%",
    alignItems: "center",
    flexWrap: "wrap",
    gap: layout.controlGap,
  },
});
export function Disclosure({
  title,
  children,
  open,
  defaultOpen,
  onOpenChange,
  keepMounted,
  variant = "plain",
  actions,
  "data-focused": focused,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title" | "onChange"> & {
  title: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  keepMounted?: boolean;
  variant?: "plain" | "row";
  actions?: ReactNode;
  "data-focused"?: boolean;
}) {
  return (
    <CollapsiblePrimitive.Root
      open={open}
      defaultOpen={defaultOpen}
      onOpenChange={onOpenChange}
      render={
        <section
          {...props}
          data-focused={focused}
          {...stylex.props(styles.root, variant === "row" && styles.row)}
        />
      }
    >
      <div data-focus-unit="" {...stylex.props(styles.header)}>
        <CollapsiblePrimitive.Trigger
          render={
            <Button
              variant="ghost"
              size="sm"
              active={focused}
              press={variant === "row" ? "none" : undefined}
              styles={
                variant === "row"
                  ? [styles.trigger, styles.rowTrigger]
                  : styles.trigger
              }
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
      </div>
      <CollapsiblePrimitive.Panel
        keepMounted={keepMounted}
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
