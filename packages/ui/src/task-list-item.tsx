import { Check } from "@flame/icons/react";
import { clsx } from "clsx";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { Icon } from "./font-scale";
import { TaskListMarkerPrimitive } from "./primitives/index";

export function TaskListItem({
  checked,
  label,
  children,
  className,
  ...props
}: ComponentPropsWithRef<"li"> & {
  checked: boolean;
  label: string;
  children: ReactNode;
}) {
  return (
    <li
      {...props}
      data-slot="task-list-item"
      className={clsx("task-list-item", className)}
    >
      <TaskListMarkerPrimitive checked={checked} label={label}>
        {checked && <Icon glyph={Check} size="sm" />}
      </TaskListMarkerPrimitive>
      {children}
    </li>
  );
}
