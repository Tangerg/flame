import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@flame/ui/tokens.stylex";
import { type ComponentPropsWithoutRef, type KeyboardEvent, type Ref } from "react";
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

// Base UI has no Tree primitive. Native divs therefore carry the tree keyboard contract.
function moveInTree(event: KeyboardEvent<HTMLDivElement>) {
  const item = (event.target as HTMLElement).closest<HTMLElement>('[role="treeitem"]');
  if (!item || !event.currentTarget.contains(item)) return;
  const items = [...event.currentTarget.querySelectorAll<HTMLElement>('[role="treeitem"]')];
  const index = items.indexOf(item);
  const branch = item.hasAttribute("aria-expanded");
  const open = item.getAttribute("aria-expanded") === "true";
  const level = Number(item.getAttribute("aria-level"));

  let target: HTMLElement | undefined;
  switch (event.key) {
    case "ArrowDown":
      target = items[index + 1];
      break;
    case "ArrowUp":
      target = items[index - 1];
      break;
    case "Home":
      target = items[0];
      break;
    case "End":
      target = items.at(-1);
      break;
    case "ArrowRight":
      if (!branch) return;
      if (!open) item.click();
      else {
        const next = items[index + 1];
        if (next && Number(next.getAttribute("aria-level")) > level) target = next;
      }
      break;
    case "ArrowLeft":
      if (branch && open) item.click();
      else
        target = items
          .slice(0, index)
          .findLast((candidate) => Number(candidate.getAttribute("aria-level")) === level - 1);
      break;
    default:
      return;
  }
  event.preventDefault();
  target?.focus();
}

type DivProps = ComponentPropsWithoutRef<"div"> & { ref?: Ref<HTMLDivElement> };

function TreeRoot({ onFocus, onKeyDown, ...props }: DivProps) {
  return (
    <div
      role="tree"
      tabIndex={-1}
      {...props}
      onFocus={(event) => {
        onFocus?.(event);
        // Keys only move from a row, so focus handed to the tree itself (such as by a retiring
        // control) enters the caller's single tab-stop row; without one the tree keeps focus.
        const root = event.currentTarget;
        if (event.defaultPrevented || event.target !== root) return;
        if (root.ownerDocument.activeElement !== root) return;
        root.querySelector<HTMLElement>('[role="treeitem"][tabindex="0"]')?.focus();
      }}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        if (!event.defaultPrevented) moveInTree(event);
      }}
    />
  );
}

type TreeItemProps = Omit<DivProps, "onClick" | "role"> & {
  level: number;
  expanded?: boolean;
  // Omitted for items that cannot be selected; branches may still be selectable for other callers.
  selected?: boolean;
  focusable: boolean;
  onActivate: () => void;
};

function TreeItem({
  className,
  style,
  level,
  expanded,
  selected,
  focusable,
  onActivate,
  onKeyDown,
  ...props
}: TreeItemProps) {
  const row = stylex.props(styles.row, type.uiMd);
  return (
    <div
      {...props}
      data-focus-proxy
      className={cn(row.className, className)}
      style={{ paddingLeft: `calc(${space.s1_5} + ${(level - 1) * LEVEL_INDENT_PX}px)`, ...style }}
      role="treeitem"
      aria-level={level}
      aria-expanded={expanded}
      aria-selected={selected}
      tabIndex={focusable ? 0 : -1}
      onClick={onActivate}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        if (event.defaultPrevented || event.target !== event.currentTarget) return;
        if (event.key !== "Enter" && event.key !== " ") return;
        event.preventDefault();
        onActivate();
      }}
    />
  );
}

function TreeGroup(props: DivProps) {
  return <div role="group" {...props} />;
}

export const Tree = { Root: TreeRoot, Item: TreeItem, Group: TreeGroup };
