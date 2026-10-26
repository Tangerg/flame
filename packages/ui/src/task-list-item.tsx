import { Check } from "@flame/icons/react";
import { clsx } from "clsx";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { Icon } from "./font-scale";

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
      <span
        data-slot="task-list-marker"
        role="checkbox"
        aria-checked={checked}
        aria-disabled="true"
        aria-label={label}
      >
        {checked && <Icon glyph={Check} size="sm" />}
      </span>
      {children}
    </li>
  );
}
