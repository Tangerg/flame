import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

export function SettingRow({
  label,
  sub,
  align = "center",
  children,
}: {
  label: string;
  sub: string;
  align?: "start" | "center";
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[minmax(0,1fr)_auto] gap-6 border-t-[length:var(--control-edge-width)] border-field px-4 py-3 first:border-t-0",
        align === "start" ? "items-start" : "items-center",
      )}
    >
      <div>
        <div className="text-ui-md text-fg">{label}</div>
        <div className="mt-1 text-ui-md leading-body text-fg-muted">{sub}</div>
      </div>
      <div className="justify-self-end">{children}</div>
    </div>
  );
}
