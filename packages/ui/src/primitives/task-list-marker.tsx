import type { ReactNode } from "react";

export function TaskListMarkerPrimitive({
  checked,
  label,
  children,
}: {
  checked: boolean;
  label: string;
  children?: ReactNode;
}) {
  return (
    <span
      data-slot="task-list-marker"
      role="checkbox"
      aria-checked={checked}
      aria-disabled="true"
      aria-label={label}
    >
      {children}
    </span>
  );
}
