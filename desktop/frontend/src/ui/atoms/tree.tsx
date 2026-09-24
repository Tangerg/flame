import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { TreePrimitive, type TreeItemPrimitiveProps } from "@/ui/primitives";

const LEVEL_INDENT_PX = 12;

const styles = stylex.create({
  row: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
    borderRadius: radius.card,
    paddingBlock: space.s1,
    paddingRight: space.s1_5,
    textAlign: "left",
    color: color.fg,
    cursor: "default",
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      ":focus-visible": surface.hover,
      ":is([aria-selected=true])": surface.selected,
      ":is([aria-selected=true]):hover": surface.selectedHover,
    },
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
});

function TreeItem({ className, style, level, ...props }: TreeItemPrimitiveProps) {
  const row = stylex.props(styles.row, type.uiMd);
  return (
    <TreePrimitive.Item
      {...props}
      level={level}
      data-chrome-focus=""
      className={cn(row.className, className)}
      style={{ paddingLeft: `calc(${space.s1_5} + ${(level - 1) * LEVEL_INDENT_PX}px)`, ...style }}
    />
  );
}

export const Tree = {
  Root: TreePrimitive.Root,
  Item: TreeItem,
  Group: TreePrimitive.Group,
};
