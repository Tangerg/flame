import type { ComponentPropsWithoutRef, KeyboardEvent, Ref } from "react";

// The WAI-ARIA tree pattern: one row sits in the tab order, the arrows walk the
// visible rows in document order, Right opens a branch or enters it, Left closes it
// or climbs to its parent. Opening and closing go through the row's own click, so a
// branch has one toggle whether it is pressed, clicked or reached by keyboard.
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
      else target = items[index + 1];
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

function TreeRoot({ onKeyDown, ...props }: DivProps) {
  return (
    <div
      role="tree"
      tabIndex={-1}
      {...props}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        if (!event.defaultPrevented) moveInTree(event);
      }}
    />
  );
}

export type TreeItemPrimitiveProps = Omit<DivProps, "onClick" | "role"> & {
  level: number;
  expanded?: boolean;
  selected: boolean;
  focusable: boolean;
  onActivate: () => void;
};

function TreeItem({
  level,
  expanded,
  selected,
  focusable,
  onActivate,
  onKeyDown,
  ...props
}: TreeItemPrimitiveProps) {
  return (
    <div
      {...props}
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

export const TreePrimitive = { Root: TreeRoot, Item: TreeItem, Group: TreeGroup };
