import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";

export type StepState = "done" | "active" | "pending";

const MARK = "grid h-4 w-4 shrink-0 place-items-center";

export function StepMark({ state }: { state: StepState }) {
  return (
    <div className={MARK}>
      {state === "done" && <Icon name="check" size="sm" className="text-success" />}
      {state === "active" && (
        <div className="relative h-3 w-3 rounded-full border-[1.5px] border-accent">
          <div className="absolute inset-0.5 animate-pulse-dot rounded-full bg-accent" />
        </div>
      )}
      {state === "pending" && (
        <div className="h-3 w-3 rounded-full border-[1.5px] border-field-strong" />
      )}
    </div>
  );
}

export function StepRow({
  state,
  className,
  children,
}: {
  state: StepState;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 py-0.5 text-ui-sm",
        state === "done" && "text-fg-faint",
        state === "active" && "font-medium text-fg",
        state === "pending" && "text-fg-muted",
        className,
      )}
    >
      <StepMark state={state} />
      <span className={cn("min-w-0 flex-1", state === "done" && "line-through")}>{children}</span>
    </div>
  );
}
